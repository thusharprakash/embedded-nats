package api

import (
	"fmt"
	"nats-dos-sdk/pkg/natsclient"

	"github.com/nats-io/nats.go"
)

// GetOrderEvents retrieves all events related to a specific order ID
func GetOrderEvents(nc *nats.Conn, orderID string) ([]string, error) {
	subject := fmt.Sprintf("order.%s", orderID)
	return natsclient.GetEvents(nc, subject)
}

// GetAllStoreEvents retrieves all events in the store
func GetAllStoreEvents(nc *nats.Conn) ([]string, error) {
	return natsclient.GetEvents(nc, "order.*")
}
