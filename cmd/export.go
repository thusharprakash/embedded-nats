package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"nats-dos-sdk/pkg/config"
	"nats-dos-sdk/pkg/natslib"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var globalCallback MessageCallback
var globalServerName string
var globalStream jetstream.Stream

var (
	existingNumbers = make(map[int]bool)
	mu              sync.Mutex
)

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


func StartNATS(serverName string,storage string, isProducer bool,callback MessageCallback){
	globalCallback = callback
	globalServerName = serverName
	cfg, err := config.LoadMobileConfig(serverName)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	natslib.SetStoragePath(storage+"/")
	server, conn, err := natslib.CreateNatsServer(cfg, true, false)
	if err != nil {
		log.Fatalf("Failed to start NATS server: %v", err)
	}

	if isProducer == true{
		go producer(context.Background(), conn)
	}

	go consumer(context.Background(), conn, cfg.ServerName,isProducer)

	defer server.Shutdown()
	defer conn.Close()

	select {} // Block forever to keep the program running
}
