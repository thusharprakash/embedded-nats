package mdnslib

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	internal "nats-dos-sdk/pkg/internals"
	"net"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/zeroconf/v2"
	"github.com/multiformats/go-multiaddr"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// MDNSService is a simple service to discover peers in the local network using mDNS
type MDNSService struct {
	host  host.Host
	peers map[string]peer.AddrInfo
	mu    sync.Mutex

	serviceName string
	peerName    string

	// The context is canceled when Close() is called.
	ctx       context.Context
	ctxCancel context.CancelFunc

	resolverWG sync.WaitGroup
	server     *zeroconf.Server

}

const (
	mdnsDomain      = "local"
	dnsaddrPrefix   = "dnsaddr="
)


// NewMDNSService initializes the mDNS service
func NewMDNSService(h host.Host,serviceTag string) *MDNSService {
	context, contextCancel := context.WithCancel(context.Background())
	return &MDNSService{
		host:  h,
		peers: make(map[string]peer.AddrInfo),
		serviceName: serviceTag,
		peerName: randomString(32 + rand.Intn(32)), // nolint:gosec
		ctx: 	 context,
		ctxCancel: contextCancel,
	}
}

func randomString(l int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	s := make([]byte, 0, l)
	for i := 0; i < l; i++ {
		s = append(s, alphabet[rand.Intn(len(alphabet))]) // nolint:gosec
	}
	return string(s)
}

// HandlePeerFound is called when a new peer is discovered
func (m *MDNSService) HandlePeerFound(pi peer.AddrInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	peerID := pi.ID.String()
	if _, exists := m.peers[peerID]; !exists {
		filteredAddrs := m.filterIPv4Addresses(pi.Addrs)
		if len(filteredAddrs) > 0 {
			pi.Addrs = filteredAddrs
			fmt.Printf("Discovered new peer: %s\n", peerID)
			for _, addr := range pi.Addrs {
				fmt.Printf("Addr  %s\n", addr)
			}
			m.peers[peerID] = pi
		}
	}
}

// filterIPv4Addresses filters out loopback and non-IPv4 addresses
func (m *MDNSService) filterIPv4Addresses(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	filtered := []multiaddr.Multiaddr{}

	for _, addr := range addrs {
		ip, err := addr.ValueForProtocol(multiaddr.P_IP4)
		if err != nil {
			continue // Not an IPv4 address
		}

		if net.ParseIP(ip).IsLoopback() {
			continue // Exclude loopback addresses
		}

		filtered = append(filtered, addr)
	}

	return filtered
}

func GetMulticastInterfaces() ([]net.Interface, error) {
	// manually get interfaces list
	ifaces, err := internal.GetNetDriver().Interfaces()
	if err != nil {
		return nil, err
	}

	// filter Multicast interfaces
	return filterMulticastInterfaces(ifaces), nil
}

func filterMulticastInterfaces(ifaces []net.Interface) []net.Interface {
	interfaces := []net.Interface{}
	for _, ifi := range ifaces {
		if (ifi.Flags & net.FlagUp) == 0 {
			continue
		}
		if (ifi.Flags & net.FlagMulticast) > 0 {
			interfaces = append(interfaces, ifi)
		}
	}

	return interfaces
}

func (s *MDNSService) Start() error {
	if err := s.startServer(); err != nil {
		return err
	}
	s.startResolver(s.ctx)
	return nil
}

// We don't really care about the IP addresses, but the spec (and various routers / firewalls) require us
// to send A and AAAA records.
func (s *MDNSService) getIPs(addrs []ma.Multiaddr) ([]string, error) {
	var ip4, ip6 string
	for _, addr := range addrs {
		network, hostport, err := manet.DialArgs(addr)
		if err != nil {
			continue
		}
		host, _, err := net.SplitHostPort(hostport)
		if err != nil {
			continue
		}
		if ip4 == "" && (network == "udp4" || network == "tcp4") {
			ip4 = host
		} else if ip6 == "" && (network == "udp6" || network == "tcp6") {
			ip6 = host
		}
	}
	ips := make([]string, 0, 2)
	if ip4 != "" {
		ips = append(ips, ip4)
	}
	if ip6 != "" {
		ips = append(ips, ip6)
	}
	if len(ips) == 0 {
		LogToNative("Didn't find any IP addresses")
		return nil, errors.New("didn't find any IP addresses")
	}
	return ips, nil
}


func (s *MDNSService) startServer() error {
	interfaceAddrs, err := s.host.Network().InterfaceListenAddresses()
	if err != nil {
		return err
	}
	addrs, err := peer.AddrInfoToP2pAddrs(&peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: interfaceAddrs,
	})
	if err != nil {
		return err
	}
	var txts []string
	for _, addr := range addrs {
		if manet.IsThinWaist(addr) { // don't announce circuit addresses
			txts = append(txts, dnsaddrPrefix+addr.String())
		}
	}

	ips, err := s.getIPs(addrs)
	if err != nil {
		return err
	}

	// manually get interfaces list
	ifaces, err := GetMulticastInterfaces()
	if err != nil {
		return err
	}
	LogToNative("Multicast interfaces found: "+ fmt.Sprint(len(ifaces)))

	server, err := zeroconf.RegisterProxy(
		s.peerName,
		s.serviceName,
		mdnsDomain,
		4001, // we have to pass in a port number here, but libp2p only uses the TXT records
		s.peerName,
		ips,
		txts,
		ifaces,
	)
	if err != nil {
		LogToNative("Failed to register proxy -> "+ err.Error());
		return err
	}
	s.server = server
	return nil
}

func (s *MDNSService) startResolver(ctx context.Context) {
	s.resolverWG.Add(2)
	entryChan := make(chan *zeroconf.ServiceEntry, 1000)
	go func() {
		defer s.resolverWG.Done()
		for entry := range entryChan {
			// We only care about the TXT records.
			// Ignore A, AAAA and PTR.
			addrs := make([]ma.Multiaddr, 0, len(entry.Text)) // assume that all TXT records are dnsaddrs
			for _, text := range entry.Text {
				if !strings.HasPrefix(text, dnsaddrPrefix) {

					LogToNative("Missing dnsaddr prefix")
					continue
				}
				addr, err := ma.NewMultiaddr(text[len(dnsaddrPrefix):])
				if err != nil {
					LogToNative("Failed to parse multiaddr -> "+ err.Error())
					continue
				}
				addrs = append(addrs, addr)
			}
			infos, err := peer.AddrInfosFromP2pAddrs(addrs...)
			if err != nil {
				LogToNative("Failed to get peer info -> "+ err.Error())
				continue
			}
			for _, info := range infos {
				 s.HandlePeerFound(info)
			}
		}
	}()
	go func() {
		// manually get interfaces list
		ifaces, err := internal.GetNetDriver().Interfaces()
		if err != nil {
			LogToNative("Zeroconf failed to get device interfaces -> "+ err.Error())
			return
		}
		// filter Multicast interfaces
		ifaces = filterMulticastInterfaces(ifaces)

		defer s.resolverWG.Done()
		if err := zeroconf.Browse(ctx, s.serviceName, mdnsDomain, entryChan, zeroconf.SelectIfaces(ifaces)); err != nil {
			LogToNative("Zeroconf browsing failed -> "+ err.Error())
		}
	}()
}

func LogToNative(message string) {
	fmt.Println(message)
}



// StartDiscovery starts mDNS discovery
func (m *MDNSService) StartDiscovery( serviceTag string) (*MDNSService,error ){
	LogToNative("Starting mDNS discovery")
	disc := NewMDNSService(m.host,serviceTag)
	LogToNative("mDNS service created")
	ifaces, err := GetMulticastInterfaces()
	LogToNative("Multicast interfaces found: "+ fmt.Sprint(len(ifaces)))
	if err != nil {
		fmt.Println("Failed to get multicast interfaces", err)
		return nil,err
	}
	LogToNative("Multicast interfaces found: "+ fmt.Sprint(len(ifaces)))

	if len(ifaces) > 0 {
		LogToNative("ifaces >0")
		if err := disc.Start(); err != nil {
			LogToNative("Failed to start mDNS discovery"+ err.Error())
			return nil,err
		}
	} else {
		fmt.Println("No multicast interfaces found")
	}

	return disc,nil
}

// GetPeers returns the list of discovered peers
func (m *MDNSService) GetPeers() map[string]peer.AddrInfo {
	// m.mu.Lock()
	// defer m.mu.Unlock()
	fmt.Printf("Returning peers: %v\n", m.peers)
	return m.peers
}
