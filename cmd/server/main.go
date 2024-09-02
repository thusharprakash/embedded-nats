package main

import (
	"context"
	"fmt"
	"log"
	"nats-dos-sdk/pkg/config"
	"nats-dos-sdk/pkg/natslib"
	"os"
	"os/signal"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func producer(ctx context.Context, nc *nats.Conn) {
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Failed to create JetStream context: %v", err)
	}

	// Retry loop to create a stream
	// Remove the declaration of the unused variable
	for i := 0; i < 5; i++ {
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:     "orders",
			MaxBytes: 1024 * 1024 * 1024,
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
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	server, conn, err := natslib.CreateNatsServer(cfg, true, false)
	if err != nil {
		log.Fatalf("Failed to start NATS server: %v", err)
	}

	if cfg.ServerName == "node1" {
		go producer(context.Background(), conn)
	}

	go consumer(context.Background(), conn, cfg.ServerName)

	defer server.Shutdown()
	defer conn.Close()

	select {} // Block forever to keep the program running
}
