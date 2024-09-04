package mdnslib

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
)

// MDNSService is a simple service to discover peers in the local network using mDNS
type MDNSService struct {
	host  host.Host
	peers map[string]peer.AddrInfo
	mu    sync.Mutex
}

// NewMDNSService initializes the mDNS service
func NewMDNSService(h host.Host) *MDNSService {
	return &MDNSService{
		host:  h,
		peers: make(map[string]peer.AddrInfo),
	}
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

// StartDiscovery starts mDNS discovery
func (m *MDNSService) StartDiscovery(ctx context.Context, serviceTag string) error {
	disc := mdns.NewMdnsService(m.host, serviceTag, m)
	return disc.Start()
}

// GetPeers returns the list of discovered peers
func (m *MDNSService) GetPeers() map[string]peer.AddrInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.peers
}
