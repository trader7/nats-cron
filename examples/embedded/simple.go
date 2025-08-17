package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/trader7/nats-cron/pkg/server"
)

// SimpleExample shows the minimal code needed to embed nats-cron
func SimpleExample() {
	fmt.Println("=== Simple Integration Example ===")
	
	// 1. Create server with default options
	srv, err := server.New(server.DefaultOptions())
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// 2. Start the server
	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// 3. Create a scheduled job
	jobDefinition := map[string]interface{}{
		"target": map[string]string{
			"subject": "my.app.task",
		},
		"payload": map[string]string{
			"data": `{"message": "Hello from scheduler!"}`,
		},
		"schedule": map[string]string{
			"every": "10s", // Every 10 seconds
		},
	}

	jobData, _ := json.Marshal(jobDefinition)
	if err := srv.GetScheduler().CreateJob(jobData); err != nil {
		log.Fatalf("Failed to create job: %v", err)
	}

	fmt.Printf("✅ NATS Cron server embedded successfully!\n")
	fmt.Printf("   - Running: %v\n", srv.IsRunning())
	fmt.Printf("   - Is Leader: %v\n", srv.IsLeader())
	fmt.Printf("   - Job created for subject: my.app.task\n")
	
	// In a real app, you would:
	// 1. Subscribe to "my.app.task" to handle the scheduled messages
	// 2. Keep the server running throughout your app's lifecycle
	// 3. Call srv.Stop() during graceful shutdown
}