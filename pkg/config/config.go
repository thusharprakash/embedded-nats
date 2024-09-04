package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds the configuration for the NATS server
type Config struct {
	ServerName   string
	ServerPort   int
	ClusterPort  int
	ClusterPeers []string
}

// LoadConfig loads the configuration from environment variables
func LoadConfig() (*Config, error) {
	serverName := os.Getenv("NATS_SERVER_NAME")
	serverPort, err := strconv.Atoi(os.Getenv("NATS_SERVER_PORT"))
	if err != nil {
		return nil, err
	}
	clusterPort, err := strconv.Atoi(os.Getenv("NATS_CLUSTER_PORT"))
	if err != nil {
		return nil, err
	}
	clusterPeers := strings.Split(os.Getenv("NATS_CLUSTER_PEERS"), ",")

	return &Config{
		ServerName:   serverName,
		ServerPort:   serverPort,
		ClusterPort:  clusterPort,
		ClusterPeers: clusterPeers,
	}, nil
}


func LoadMobileConfig(serverName string) (*Config, error) {
	serverPort, err := strconv.Atoi("4223")
	clusterPeer :="nats://192.168.1.18:6222"
	if err != nil {
		return nil, err
	}
	clusterPort, err := strconv.Atoi("6222")
	if err != nil {
		return nil, err
	}
	clusterPeers := strings.Split(clusterPeer, ",")

	return &Config{
		ServerName:   serverName,
		ServerPort:   serverPort,
		ClusterPort:  clusterPort,
		ClusterPeers: clusterPeers,
	}, nil
}