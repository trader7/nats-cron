package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/trader7/nats-cron/pkg/server"
)

func main() {
	// Handle help flag
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help") {
		printUsage()
		return
	}

	// Create server with default options (uses environment variables)
	srv, err := server.New(server.DefaultOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("Shutting down...")
		cancel()
	}()

	// Start the server
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	srv.Wait()

	// Stop the server
	if err := srv.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping server: %v\n", err)
	}
}

func printUsage() {
	fmt.Println("NATS Cron Scheduler")
	fmt.Println()
	fmt.Println("A distributed cron scheduler that publishes messages to NATS subjects on schedule.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  nats-cron [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h, --help    Show this help")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  NATS_URL      NATS server URL (default: nats://localhost:4222)")
	fmt.Println("  LOG_LEVEL     Log level: debug, info, warn, error (default: info)")
	fmt.Println("  INSTANCE_ID   Unique instance identifier (default: hostname)")
	fmt.Println()
	fmt.Println("Management:")
	fmt.Println("  Use 'nats-cron' command to manage jobs")
	fmt.Println("  Use 'nats micro list' to see running instances")
}
