package sdk

import (
	"context"
	"fmt"
	"log"

	"nats-dos-sdk/pkg/config"
	"nats-dos-sdk/pkg/natslib"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Sdk represents the NATS-based messaging Sdk.
type Sdk struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	server *server.Server
}

// NewSdk initializes the Sdk with the provided configuration.
func NewSdk(cfg config.Config) *Sdk {
	// Load configuration
	conf, err := config.LoadConfig()
	if err != nil {
		fmt.Errorf("failed to load config: %v", err)

		return nil
	}

	// Start the NATS server using natslib
	server, conn, err := natslib.CreateNatsServer(conf, true, false)
	if err != nil {
		fmt.Errorf("failed to start NATS server: %v", err)
		return nil
	}

	// Create a JetStream context
	js, err := jetstream.New(conn)
	if err != nil {
		fmt.Errorf("failed to create JetStream context: %v", err)
		return nil
	}

	// Return the Sdk instance
	return &Sdk{
		nc:     conn,
		js:     js,
		server: server,
	}
}

// CreateStream creates or updates a stream.
func (sdk *Sdk) CreateStream(ctx context.Context, streamName string, subjects []string) error {
	_, err := sdk.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     streamName,
		MaxBytes: 1024 * 1024 * 1024, // 1 GB
		Subjects: subjects,
	})
	if err != nil {
		return fmt.Errorf("failed to create stream: %v", err)
	}
	return nil
}

// PublishMessage publishes a message to a subject.
func (sdk *Sdk) PublishMessage(ctx context.Context, subject string, message string) error {
	_, err := sdk.js.Publish(ctx, subject, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to publish message: %v", err)
	}
	log.Println("Message published:", message)
	return nil
}

// ConsumeMessages consumes messages from a specified stream and consumer.
func (sdk *Sdk) ConsumeMessages(ctx context.Context, streamName, consumerName string) error {
	// Retrieve the stream
	stream, err := sdk.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream: %v", err)
	}

	// Create or update the consumer
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:    consumerName,
		Durable: consumerName,
	})
	if err != nil {
		return fmt.Errorf("failed to create consumer: %v", err)
	}

	// Consume messages
	cctx, err := consumer.Consume(func(msg jetstream.Msg) {
		log.Println("Received message:", string(msg.Subject()), string(msg.Data()))
		msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("failed to consume messages: %v", err)
	}
	defer cctx.Stop()

	return nil
}

// Close shuts down the Sdk and closes the NATS connection.
func (sdk *Sdk) Close() {
	if sdk.server != nil {
		sdk.server.Shutdown()
	}
	if sdk.nc != nil {
		sdk.nc.Close()
	}
}
