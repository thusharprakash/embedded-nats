package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"nats-dos-sdk/pkg/config"
	internal "nats-dos-sdk/pkg/internals"
	"nats-dos-sdk/pkg/mdnslib"
	"nats-dos-sdk/pkg/natslib"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

var globalCallback MessageCallback
var globalServerName string
var globalStream jetstream.Stream

var (
	existingNumbers = make(map[int]bool)
	mu              sync.Mutex
)

func setupLibp2pMDNS() (*mdnslib.MDNSService, error) {
	h, err := libp2p.New(libp2p.Security(noise.ID, noise.New),
	libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
	if err != nil {
		fmt.Println("ERROR IN LIBP2P HOST",err)
		return nil, fmt.Errorf("failed to create libp2p host: %v", err)
	}

	fmt.Println("LIBP2P HOST CREATED")

	mdnsService := mdnslib.NewMDNSService(h, "nats-discovery")
	service,err := mdnsService.StartDiscovery( "nats-discovery")
	if err != nil {
		return nil, fmt.Errorf("failed to start mDNS discovery: %v", err)
	}
	fmt.Println("MDNS SERVICE RETURNED")

	return service, nil
}

func createNatsURLsFromAddrInfo(addrInfo multiaddr.Multiaddr) (string, error) {
	log.Printf("Processing multiaddr: %s", addrInfo.String())

	// Initialize variables for IP and port
	var ip string
	port := "4222" // Default NATS port, always set to 4222

	// Split the multiaddress into its components
	maSplit := multiaddr.Split(addrInfo)

	// Iterate through the components and extract IP and TCP information
	for _, addr := range maSplit {
		log.Printf("Inspecting component: %s", addr.String())

		switch addr.Protocols()[0].Code {
		case multiaddr.P_IP4:
			// Use String(), then strip the `/ip4/` part to get the actual IP
			ip = strings.TrimPrefix(addr.String(), "/ip4/")
			log.Printf("Found IPv4 address: %s", ip)
		case multiaddr.P_TCP:
			// Use String(), then strip the `/tcp/` part to get the actual port
			port = strings.TrimPrefix(addr.String(), "/tcp/")
			log.Printf("Found TCP port: %s", port)
		}
	}

	// Check if an IP address was extracted
	if ip == "" {
		return "", fmt.Errorf("no valid IPv4 address found in multiaddr: %s", addrInfo.String())
	}

	// Construct the NATS URL with the default or extracted port
	natsURL := fmt.Sprintf("nats://%s:%s", ip, "6222")
	log.Printf("Constructed NATS URL: %s", natsURL)

	return natsURL, nil
}

func producer(ctx context.Context, nc *nats.Conn) {
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Failed to create JetStream context: %v", err)
	}

	// Retry loop to create a stream
	// Remove the declaration of the unused variable
	for i := 0; i < 30; i++ {
		stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:     "orders",
			MaxBytes: 1024 * 1024 * 1024,
			Subjects: []string{"orders.>"},
		})
		if stream != nil {
			globalStream = stream
			fmt.Println("stream created",stream,stream.CachedInfo().State)
		}
		
		if err == nil {
			break
		}
		log.Printf(" Producer ==> Failed to create stream, retrying... (%d/5): %v", i+1, err)
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		log.Fatalf("Producer ==> Failed to create stream after retries: %v", err)
	}

	subject := "orders.>"
	i := 0
	for {
		select {
		case <-ctx.Done():
			log.Println("exiting from producer")
			return
		default:
			i += 1
			message := fmt.Sprintf("message %v from %s", i,globalServerName)
			time.Sleep(3 * time.Second)

			ack, err := js.Publish(ctx, subject, []byte(message))
			if err != nil {
				log.Println("Failed to publish message:", err)
			} else {
				log.Println(message)
			}
			if ack != nil {
				log.Println("Ack:", ack.Stream, ack.Sequence)
			}
		}
	}
}

func consumer(ctx context.Context, nc *nats.Conn, name string,isProducer bool) {
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Failed to create JetStream context: %v", err)
	}
	var stream jetstream.Stream
	time.Sleep(5 * time.Second)
	for {
		if(isProducer == true && globalStream == nil){
			continue	
		}
		fmt.Println("stream found",globalStream)
		stream, err = js.Stream(ctx, "orders")
		if err == nil {
			break
		}
		log.Printf("Consumer ==> Failed to get stream, retrying %v", err)
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		log.Fatalf("Consumer ==> Failed to get stream after retries: %v", err)
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:    "orders-consumer_" + name,
		Durable: "orders-consumer_" + name,
	})
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", wrapErrorWithStackTrace(err))
	}

	cctx, err := consumer.Consume(func(msg jetstream.Msg) {
		log.Println("Received message", string(msg.Subject()), string(msg.Data()))
		globalCallback.OnMessageReceived("Received message " + string(msg.Subject()) + " " + string(msg.Data()))
		msg.Ack()
	})
	if err != nil {
		log.Fatalf("Failed to consume messages: %v", err)
	}

	defer cctx.Stop()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}

func wrapErrorWithStackTrace(err error) error {
    buf := make([]byte, 1024)
    runtime.Stack(buf, false)
    return errors.New(string(buf) + "\n" + err.Error())
}


func StartNATS(serverName string,storage string, isProducer bool,mdnsLocker NativeMDNSLockerDriver,
	netDriver NativeNetDriver,callback MessageCallback){
	globalCallback = callback
	globalServerName = serverName

	if netDriver != nil {
		logger, _ := zap.NewDevelopment()
		inet := &inet{
			net:    netDriver,
			logger: logger,
		}

		internal.SetNetDriver(inet)
		manet.SetNetInterface(inet)
	}

	mdnsLocked := false

	if mdnsLocker != nil {
		mdnsLocker.Lock()
		mdnsLocked = true
	}
	mdnsService, err := setupLibp2pMDNS()
	if err != nil {
		log.Fatalf("Failed to setup libp2p mDNS: %v", err)
	}

	fmt.Println("Timer starts")
	time.Sleep(5 * time.Second) // Wait for peer discovery
	peers := mdnsService.GetPeers()

	fmt.Println("Peers found",peers)
	
	fmt.Println("Loading config")
	cfg, err := config.LoadMobileConfig(serverName)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Println("Config loaded")
	natslib.SetStoragePath(storage+"/")

	fmt.Println("Creating peers")
	for peerID, addrInfo := range peers {
		log.Printf("Peer discovered: %s at %s\n", peerID, addrInfo.Addrs)
		for _, addr := range addrInfo.Addrs {
			natsURL, err := createNatsURLsFromAddrInfo(addr)
			if err != nil {
				log.Printf("Failed to create NATS URL for peer %s: %v", peerID, err)
				continue
			}
			log.Println("Adding peer to cluster:", natsURL)
			// Check if the peer is already in the list
			found := false
			for _, p := range cfg.ClusterPeers {
				if p == natsURL {
					found = true
					break
				}
			}
			if found {
				log.Println("Peer already in cluster, skipping...")
				continue
			}

			cfg.ClusterPeers = append(cfg.ClusterPeers, natsURL)
		}
	}
	fmt.Println("Peers created")

	server, conn, err := natslib.CreateNatsServer(cfg, true, false)
	if err != nil {
		log.Fatalf("Failed to start NATS server: %v", err)
	}
	fmt.Println("NATS server started")
	if mdnsLocked && mdnsLocker != nil {
		mdnsLocker.Unlock()
	}
	fmt.Println("Producer starts")
	if isProducer == true{
		go producer(context.Background(), conn)
	}
	fmt.Println("Consumer starts")
	go consumer(context.Background(), conn, cfg.ServerName,isProducer)

	defer server.Shutdown()
	defer conn.Close()

	select {} // Block forever to keep the program running
}
