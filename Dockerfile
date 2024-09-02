# Start with a Golang base image
FROM golang:1.21-alpine as build

# Set the working directory inside the container
WORKDIR /app

# Copy the Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go application
RUN go build -o main ./cmd/server

# Start a new stage for the runtime environment
FROM alpine:latest

# Set the working directory inside the container
WORKDIR /root/

# Copy the built application from the previous stage
COPY --from=build /app/main .

# Command to run the application
CMD ["./main"]
