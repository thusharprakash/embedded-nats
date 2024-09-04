package natslib

import (
	"fmt"
	"nats-dos-sdk/pkg/config"
	"net/url"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

<<<<<<< HEAD
=======
var storage string = "./"

func SetStoragePath(path string) {
	storage = path
}

// Create a nats server with the given configuration
// set defauilt value for inProcess to false
>>>>>>> 1d5e2ff (Add working mobile support)
func CreateNatsServer(cfg *config.Config, isLogEnabled bool, inProcess bool) (*server.Server, *nats.Conn, error) {
	// Parse the leaf node URLs for hub1 through hub3
	leafUrls := make([]*url.URL, 0)
	leafNodes := []string{"nats://oolio:password@nats-hub1:7422", "nats://oolio:password@nats-hub2:7422", "nats://oolio:password@nats-hub3:7422"}
	for _, leaf := range leafNodes {
		leafUrl, err := url.Parse(leaf)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing leaf url: %v", err)
		}

		leafUrls = append(leafUrls, leafUrl)
	}

	// leafUrl, err := url.Parse("nats://nats-hub1:7422")
	// if err != nil {
	// 	return nil, nil, fmt.Errorf("error parsing leaf url: %v", err)
	// }

	// Configure leaf node options
	leafNodeOptions := server.LeafNodeOpts{
		Username: "oolio",
		Password: "password",
		Remotes: []*server.RemoteLeafOpts{
			{

				URLs: leafUrls,
			},
		},
	}

	// Set up cluster routes
	roueUrls := make([]*url.URL, 0)
	for _, peer := range cfg.ClusterPeers {
		peerUrl, err := url.Parse(peer)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing peer url: %v", err)
		}
		roueUrls = append(roueUrls, peerUrl)
	}

	// Create server options
	opts := &server.Options{
		ServerName:      cfg.ServerName,
		Host:            "0.0.0.0",
		Port:            cfg.ServerPort,
		Username:        "admin",
		Password:        "password",
		DontListen:      inProcess,
		JetStream:       true,
		JetStreamDomain: "embedded",
		LeafNode:        leafNodeOptions,
		Routes:          roueUrls,
		Debug: 		 true,
		StoreDir:        storage,
		Cluster: server.ClusterOpts{
			Name: "nats-cluster",
			Port: cfg.ClusterPort,
		},
	}

	// Start the NATS server
	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("error starting server: %v", err)
	}

	// Enable logging if required
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
