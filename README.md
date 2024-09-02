# NATS DOS SDK

## Overview
This project is a simple event-driven SDK using embedded NATS servers. It allows for pushing and retrieving events, and provides APIs for accessing order-specific events and all store events.

## File Structure
```
nats-dos-sdk/
│
├── cmd/
│   └── server/
│       └── main.go                # Entry point for the NATS embedded server
│
├── pkg/
│   ├── natsclient/
│   │   ├── client.go              # NATS client for pushing and retrieving events
│   │   └── client_test.go         # Unit tests for NATS client
│   ├── api/
│   │   ├── events.go              # API for retrieving order events and store events
│   │   └── events_test.go         # Unit tests for the API
│   └── config/
│       └── config.go              # Configuration loading from environment variables
│
├── Dockerfile                     # Dockerfile for building the Go application
├── docker-compose.yml             # Docker Compose setup for running the project
├── go.mod                         # Go module file
├── go.sum                         # Go dependencies file
└── README.md                      # Project documentation
```

## How to Run

1. **Build and Run the Docker Containers:**

   ```
   docker-compose up --build
   ```

2. **Testing the Setup:**

   - Use the provided APIs to push and retrieve events through the NATS servers.

```
gomobile bind -target=android -o nats_sdk.aar ./sdk

```
