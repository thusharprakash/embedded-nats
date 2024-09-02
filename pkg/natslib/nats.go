package natslib

import (
	"fmt"
	"log"
	"nats-dos-sdk/pkg/config"
	"net/url"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// Create a nats server with the given configuration
// set defauilt value for inProcess to false
func CreateNatsServer(cfg *config.Config, isLogEnabled bool, inProcess bool) (*server.Server, *nats.Conn, error) {
	// Manually configure the server options

	// Create the leaf node url for the hub
	leafUrl, err := url.Parse("nats-leaf://nats-hub:7422")
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing leaf url: %v", err)
	}

	// Leaf node options
	leafNodeOptions := server.LeafNodeOpts{
		Remotes: []*server.RemoteLeafOpts{
			{
				URLs: []*url.URL{leafUrl},
			},
		},
	}
	// Create the server options
	log.Printf("Leaf node options: %v", leafNodeOptions)

	roueUrls := make([]*url.URL, 0)
	for _, peer := range cfg.ClusterPeers {
		peerUrl, err := url.Parse(peer)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing peer url: %v", err)
		}
		roueUrls = append(roueUrls, peerUrl)
	}
	opts := &server.Options{
		ServerName:      cfg.ServerName,
		Host:            "0.0.0.0",
		Port:            cfg.ServerPort,
		Username:        "admin",
		Password:        "password",
		DontListen:      inProcess,
		JetStream:       true,
		JetStreamDomain: "enbedded",
		// LeafNode:        leafNodeOptions,
		Routes: roueUrls,
		Cluster: server.ClusterOpts{
			Name: "nats-cluster",
			Port: cfg.ClusterPort,
		},
	}
	// Start the NATS server with the generated options
	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("error starting server: %v", err)
	}

	//Setup loger for the server if needed
	if isLogEnabled {
		ns.ConfigureLogger()
	}

	go ns.Start()

	// Wait for the server to be ready for connections
	ns.ReadyForConnections(5 * time.Second)

	clientOps := []nats.Option{nats.UserInfo("admin", "password")}
	if inProcess {
		clientOps = append(clientOps, nats.InProcessServer(ns))
	}

	// Connect to the embedded NATS server
	nc, err := nats.Connect(ns.ClientURL(), clientOps...)
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to server: %v", err)
	}

	return ns, nc, nil

}
