package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/trader7/nats-cron/pkg/server"
	"github.com/trader7/nats-cron/pkg/scheduler"
	"go.uber.org/zap"
)

func main() {
	// Example 1: Basic embedding with default options
	fmt.Println("=== Example 1: Basic Embedding ===")
	basicEmbedding()

	fmt.Println("\n=== Example 2: Custom Configuration ===")
	customConfiguration()

	fmt.Println("\n=== Example 3: Reusing Existing NATS Connection ===")
	reusingNATSConnection()

	fmt.Println("\n=== Example 4: Embedded Mode (No Micro Service) ===")
	embeddedMode()
}

// Example 1: Basic embedding with default configuration
func basicEmbedding() {
	// Create server with default options
	srv, err := server.New(server.DefaultOptions())
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start the server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Printf("Server running: %v, Is Leader: %v\n", srv.IsRunning(), srv.IsLeader())

	// Create a simple job
	jobDef := map[string]interface{}{
		"target": map[string]string{
			"subject": "example.heartbeat",
		},
		"payload": map[string]string{
			"data": "ping",
		},
		"schedule": map[string]string{
			"every": "5s",
		},
	}

	jobData, _ := json.Marshal(jobDef)
	if err := srv.GetScheduler().CreateJob(jobData); err != nil {
		log.Printf("Failed to create job: %v", err)
	} else {
		fmt.Println("Created heartbeat job")
	}

	// Let it run for a bit
	time.Sleep(2 * time.Second)

	// Stop the server
	if err := srv.Stop(); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	fmt.Println("Basic embedding example completed")
}

// Example 2: Custom configuration
func customConfiguration() {
	// Create custom logger
	logger, _ := zap.NewDevelopment()

	// Create custom options
	opts := &server.Options{
		NATSURL:              "nats://localhost:4222",
		ServiceName:          "my-app-scheduler",
		ServiceVersion:       "2.0.0",
		InstanceID:           "my-app-instance-1",
		JobsBucketName:       "my_app_jobs",
		ElectionBucketName:   "my_app_leader",
		Logger:               logger,
		EnableMicroService:   true,
		EnableLeaderElection: true,
		ElectionTTL:          15 * time.Second,
		ShutdownTimeout:      10 * time.Second,
	}

	// Create server with custom options
	srv, err := server.New(opts)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Printf("Custom server running with service name: %s\n", opts.ServiceName)

	time.Sleep(1 * time.Second)

	if err := srv.Stop(); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	fmt.Println("Custom configuration example completed")
}

// Example 3: Reusing existing NATS connection
func reusingNATSConnection() {
	// Connect to NATS (your app might already have this)
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Subscribe to scheduler messages in your app
	sub, _ := nc.Subscribe("myapp.tasks.*", func(msg *nats.Msg) {
		fmt.Printf("Received scheduled task: %s -> %s\n", msg.Subject, string(msg.Data))
	})
	defer sub.Unsubscribe()

	// Create server options using existing connection
	opts := server.DefaultOptions()
	opts.NATSConn = nc // Reuse existing connection
	opts.ServiceName = "myapp-with-scheduler"
	opts.JobsBucketName = "myapp_jobs"

	srv, err := server.New(opts)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Create a job that publishes to our subscription
	jobData, _ := json.Marshal(map[string]interface{}{
		"target": map[string]string{"subject": "myapp.tasks.cleanup"},
		"payload": map[string]string{"data": `{"action":"cleanup","table":"sessions"}`},
		"schedule": map[string]string{"every": "3s"},
	})

	if err := srv.GetScheduler().CreateJob(jobData); err != nil {
		log.Printf("Failed to create job: %v", err)
	} else {
		fmt.Println("Created cleanup job")
	}

	// Let it run to see messages
	time.Sleep(4 * time.Second)

	if err := srv.Stop(); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	fmt.Println("NATS connection reuse example completed")
}

// Example 4: Embedded mode without micro service
func embeddedMode() {
	// Create options for embedded mode
	opts := server.DefaultOptions()
	opts.EnableMicroService = false // Disable NATS Micro service
	opts.ServiceName = "embedded-scheduler"
	opts.JobsBucketName = "embedded_jobs"

	srv, err := server.New(opts)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Println("Running in embedded mode (no micro service)")

	// Direct scheduler access for job management
	sched := srv.GetScheduler()

	// Create jobs directly
	jobData, _ := json.Marshal(map[string]interface{}{
		"target":   map[string]string{"subject": "embedded.report"},
		"payload":  map[string]string{"data": "daily_report"},
		"schedule": map[string]string{"every": "2s"},
	})

	if err := sched.CreateJob(jobData); err != nil {
		log.Printf("Failed to create job: %v", err)
	}

	// List jobs
	jobs, err := sched.GetJobs()
	if err == nil {
		fmt.Printf("Active jobs: %d\n", len(jobs))
		for _, job := range jobs {
			fmt.Printf("  - %s (next: %s)\n", job.Subject, job.NextRun.Format("15:04:05"))
		}
	}

	time.Sleep(3 * time.Second)

	if err := srv.Stop(); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	fmt.Println("Embedded mode example completed")
}

// Advanced example showing integration in a real application
func realWorldExample() {
	fmt.Println("\n=== Real World Integration Example ===")

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

	// Your application's NATS connection
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Setup your application's message handlers
	nc.Subscribe("reports.daily", handleDailyReport)
	nc.Subscribe("cleanup.database", handleDatabaseCleanup)
	nc.Subscribe("monitoring.healthcheck", handleHealthCheck)

	// Configure embedded scheduler
	opts := &server.Options{
		NATSConn:             nc,
		ServiceName:          "myapp",
		ServiceVersion:       "1.2.3",
		InstanceID:           "myapp-prod-1",
		JobsBucketName:       "myapp_scheduled_jobs",
		EnableMicroService:   true,
		EnableLeaderElection: true,
	}

	// Create and start scheduler
	scheduler, err := server.New(opts)
	if err != nil {
		log.Fatalf("Failed to create scheduler: %v", err)
	}

	if err := scheduler.Start(ctx); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	// Setup initial jobs
	setupJobs(scheduler.GetScheduler())

	fmt.Println("Application with embedded scheduler running...")
	fmt.Println("Press Ctrl+C to shutdown")

	// Your application logic here...
	scheduler.Wait()

	if err := scheduler.Stop(); err != nil {
		log.Printf("Error stopping scheduler: %v", err)
	}
}

func handleDailyReport(msg *nats.Msg) {
	fmt.Printf("Generating daily report at %s\n", time.Now().Format("15:04:05"))
}

func handleDatabaseCleanup(msg *nats.Msg) {
	fmt.Printf("Running database cleanup at %s\n", time.Now().Format("15:04:05"))
}

func handleHealthCheck(msg *nats.Msg) {
	fmt.Printf("Health check at %s\n", time.Now().Format("15:04:05"))
}

func setupJobs(sched *scheduler.Scheduler) {
	jobs := []map[string]interface{}{
		{
			"target":   map[string]string{"subject": "reports.daily"},
			"payload":  map[string]string{"data": `{"type":"sales","format":"pdf"}`},
			"schedule": map[string]string{"cron": "0 9 * * *"}, // 9 AM daily
		},
		{
			"target":   map[string]string{"subject": "cleanup.database"},
			"payload":  map[string]string{"data": `{"table":"sessions","older_than":"7d"}`},
			"schedule": map[string]string{"cron": "0 2 * * *"}, // 2 AM daily
		},
		{
			"target":   map[string]string{"subject": "monitoring.healthcheck"},
			"payload":  map[string]string{"data": `{"endpoints":["api","database","cache"]}`},
			"schedule": map[string]string{"every": "30s"},
		},
	}

	for _, job := range jobs {
		jobData, _ := json.Marshal(job)
		if err := sched.CreateJob(jobData); err != nil {
			log.Printf("Failed to create job %s: %v", job["target"].(map[string]string)["subject"], err)
		}
	}

	fmt.Println("Setup 3 scheduled jobs")
}