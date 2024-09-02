package main

import (
	"context"
	"fmt"
	"log"
	"nats-dos-sdk/pkg/config"
	"nats-dos-sdk/pkg/natsclient"
	"nats-dos-sdk/pkg/natslib"

	"github.com/nats-io/nats.go"
)

func main_old() {
	// Load configuration from environment variables

	ctx := context.Background()
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Start the NATS server with dynamic routes
	server, conn, err := natsclient.StartNATSServer(cfg)
	if err != nil {
		log.Fatalf("Failed to start NATS server: %v", err)
	}
	defer server.Shutdown()
	defer conn.Close()

	// Example: push and get events
	// err = natsclient.PushEvent(conn, "order.created", "Order ID: 12345")
	// if err != nil {
	// 	log.Fatalf("Failed to push event: %v", err)
	// }

	// events, err := natsclient.GetEvents(conn, "order.created")
	// if err != nil {
	// 	log.Fatalf("Failed to get events: %v", err)
	// }

	// go func() {
	// 	for event := range events {
	// 		log.Printf("Received event: %s", event)
	// 	}
	// }()

	// Use the NATS connection to publish and subscribe to messages
	// Use the node1 to publish messages every 3 seconds
	if cfg.ServerName == "node1" {
		go producer(ctx, conn)
	}

	// Use the  all the nodes to consume messages
	go consumer(ctx, conn)

	//

	// Keep the main function running to allow asynchronous processing
	select {} // Block forever to keep the program running

}

func producer(ctx context.Context, nc *nats.Conn) {

	// listen for messaages on this subject
	subject := "logs"

	i := 0

	for {
		select {
		case <-ctx.Done():
			log.Println("exiting from producer")
			return
		default:
			i += 1
			message := fmt.Sprintf("message %v", i)

			// Publish the message to the nats server
			err := nc.Publish(subject, []byte(message))
			if err != nil {
				log.Println("Failed to publish message:", err)
			} else {
				log.Println(message)
			}
		}

	}
}

func consumer(ctx context.Context, nc *nats.Conn) {
	// listen for messaages on this subject
	subject := "logs"
	messages := make(chan *nats.Msg, 1000)

	// we're subscribing to the subject
	// and assigning our channel as reference to receive messages there
	subscription, err := nc.ChanSubscribe(subject, messages)
	if err != nil {
		log.Fatal("Failed to subscribe to subject:", err)
	}

	defer func() {
		subscription.Unsubscribe()
		close(messages)
	}()

	log.Println("Subscribed to", subject)

	for {
		select {
		case <-ctx.Done():
			log.Println("exiting from consumer")
			return
		case msg := <-messages:
			log.Println("received", string(msg.Data))
		}
	}
}

func main() {

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Start the NATS server with dynamic routes
	server, conn, err := natslib.CreateNatsServer(cfg, true, false)
	if err != nil {
		log.Fatalf("Failed to start NATS server: %v", err)
	}

	// If node is node1, start the producer
	if cfg.ServerName == "node1" {
		go producer(context.Background(), conn)
	}

	// Start the consumer
	go consumer(context.Background(), conn)

	defer server.Shutdown()
	defer conn.Close()

	// Keep the main function running to allow asynchronous processing
	select {} // Block forever to keep the program running

}
