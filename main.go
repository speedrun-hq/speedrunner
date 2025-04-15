package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/speedrun-hq/fulfiller/pkg/config"
	"github.com/speedrun-hq/fulfiller/pkg/fulfiller"
)

func main() {
	// Load configuration from environment variables
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create the fulfiller service
	service, err := fulfiller.NewService(cfg)
	if err != nil {
		log.Fatalf("Failed to create fulfiller service: %v", err)
	}

	// Set up context with cancellation on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalCh
		log.Println("Received termination signal, shutting down gracefully...")
		cancel()
	}()

	// Start the service
	log.Println("Starting the fulfiller service...")
	service.Start(ctx)
}
