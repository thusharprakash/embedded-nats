package main

import (
	"context"
	"fmt"
	"log"
	"nats-dos-sdk/pkg/config"
	"nats-dos-sdk/pkg/mdnslib"
	"nats-dos-sdk/pkg/natslib"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/multiformats/go-multiaddr"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func setupLibp2pMDNS(ctx context.Context) (*mdnslib.MDNSService, error) {
	h, err := libp2p.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %v", err)
	}

	mdnsService := mdnslib.NewMDNSService(h)
	err = mdnsService.StartDiscovery(ctx, "nats-discovery")
	if err != nil {
		return nil, fmt.Errorf("failed to start mDNS discovery: %v", err)
	}

	return mdnsService, nil
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
	for i := 0; i < 5; i++ {
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:     "orders",
			MaxBytes: 1024 * 1024 * 1024,
			Storage:  jetstream.FileStorage,
			Subjects: []string{"orders.>"},
		})
		if err == nil {
			break
		}
		log.Printf("Failed to create stream, retrying... (%d/5): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to create stream after retries: %v", err)
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
			message := fmt.Sprintf("message %v", i)
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

func consumer(ctx context.Context, nc *nats.Conn, name string) {
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Failed to create JetStream context: %v", err)
	}

	var stream jetstream.Stream
	for i := 0; i < 5; i++ {
		stream, err = js.Stream(ctx, "orders")
		if err == nil {
			break
		}
		log.Printf("Failed to get stream, retrying... (%d/5): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to get stream after retries: %v", err)
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:    "orders-consumer_" + name,
		Durable: "orders-consumer_" + name,
	})
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	}

	cctx, err := consumer.Consume(func(msg jetstream.Msg) {
		log.Println("Received message", string(msg.Subject()), string(msg.Data()))
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

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mdnsService, err := setupLibp2pMDNS(ctx)
	if err != nil {
		log.Fatalf("Failed to setup libp2p mDNS: %v", err)
	}

	time.Sleep(15 * time.Second) // Wait for peer discovery
	peers := mdnsService.GetPeers()
	if len(peers) == 0 {
		log.Println("No peers discovered, exiting...")
		return
	}

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

	server, conn, err := natslib.CreateNatsServer(cfg, true, false)
	if err != nil {
		log.Fatalf("Failed to start NATS server: %v", err)
	}

	if cfg.ServerName == "node1" {
		go producer(ctx, conn)
	}

	go consumer(ctx, conn, cfg.ServerName)

	defer server.Shutdown()
	defer conn.Close()

	select {} // Block forever to keep the program running
}
