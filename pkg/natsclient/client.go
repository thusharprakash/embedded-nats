package natsclient

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"nats-dos-sdk/pkg/config"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// StartNATSServer starts the embedded NATS server
func StartNATSServer(cfg *config.Config) (*server.Server, *nats.Conn, error) {
	// Manually configure the server options

	fmt.Println(cfg.ServerName)
	cfgUrl, err := RemovePortFromURL(cfg.ClusterPeers[0])
	if err != nil {
		return nil, nil, fmt.Errorf("error removing port from URL: %v", err)
	}

	fmt.Println(cfgUrl)
	opts := &server.Options{
		ServerName: cfg.ServerName,
		Host:       "0.0.0.0",
		Port:       cfg.ServerPort,
		Username:   "admin",
		Password:   "password",
		Cluster: server.ClusterOpts{
			Name: "nats-cluster",
			Port: cfg.ClusterPort,
			// Routes: server.RoutesFromStrings(cfg.ClusterPeers[1:]...),

		},
	}

	// Start the NATS server with the generated options
	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("error starting server: %v", err)
	}

	ns.ConfigureLogger()
	go ns.Start()

	// Wait for the server to be ready for connections
	if !ns.ReadyForConnections(5 * time.Second) {
		return nil, nil, fmt.Errorf("server did not become ready in time")
	}

	log.Printf("NATS server started on port %d", opts.Port)

	// Connect to the embedded NATS server
	nc, err := nats.Connect(ns.ClientURL(), nats.UserInfo("admin", "password"))
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to server: %v", err)
	}

	return ns, nc, nil
}

// PushEvent sends an event to a specified subject in NATS
func PushEvent(nc *nats.Conn, subject string, message string) error {
	if nc == nil {
		return fmt.Errorf("NATS connection not established")
	}
	fmt.Println("Publishing message", message+" to subject", subject)
	return nc.Publish(subject, []byte(message))
}

// GetEvents subscribes to a subject and sends received messages through a channel
func GetEvents(nc *nats.Conn, subject string) (<-chan string, error) {
	events := make(chan string, 512) // Buffered channel to handle incoming messages

	fmt.Println("Subscribing to subject", subject)

	_, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		fmt.Println("Received message", string(msg.Data))
		events <- string(msg.Data) // Send the message data to the channel
	})
	if err != nil {
		return nil, err
	}

	return events, nil
}

func RemovePortFromURL(inputURL string) (string, error) {
	// Parse the URL
	u, err := url.Parse(inputURL)
	if err != nil {
		return "", fmt.Errorf("error parsing URL: %v", err)
	}

	// Split the host part to separate the hostname and port
	host := u.Hostname()
	port := u.Port()

	// If a port exists, remove it
	if port != "" {
		u.Host = host // Set the host without the port
	}

	// Reconstruct the URL without the port
	// Note: RawPath and Fragment parts of the URL are preserved
	u.RawPath = strings.TrimSuffix(u.RawPath, ":"+port)

	return u.String(), nil
}
