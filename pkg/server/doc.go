// Package server provides an embeddable NATS Cron scheduler that can be integrated
// directly into Go applications.
//
// The server package allows you to embed the NATS Cron functionality as a library
// rather than running it as a separate process. This provides several benefits:
//
//   - No separate process management required
//   - Shared NATS connections with your application
//   - Programmatic configuration and control
//   - Direct access to the scheduler API
//   - Graceful integration with your application lifecycle
//
// # Basic Usage
//
// The simplest way to embed NATS Cron:
//
//	srv, err := server.New(server.DefaultOptions())
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	ctx := context.Background()
//	srv.Start(ctx)
//	defer srv.Stop()
//
//	// Create jobs using the scheduler
//	sched := srv.GetScheduler()
//	jobData, _ := json.Marshal(jobDefinition)
//	sched.CreateJob(jobData)
//
// # Custom Configuration
//
// You can customize the server behavior using Options:
//
//	opts := &server.Options{
//		ServiceName:          "my-app-scheduler",
//		JobsBucketName:       "my_app_jobs",
//		EnableMicroService:   false,  // Disable API endpoints
//		EnableLeaderElection: true,   // Enable distributed scheduling
//	}
//
//	srv, err := server.New(opts)
//
// # Reusing NATS Connections
//
// If your application already has a NATS connection, you can reuse it:
//
//	nc, _ := nats.Connect("nats://localhost:4222")
//
//	opts := server.DefaultOptions()
//	opts.NATSConn = nc
//
//	srv, err := server.New(opts)
//
// # Leader Election
//
// When EnableLeaderElection is true (default), multiple server instances can run
// concurrently with automatic leader election. Only the leader will schedule and
// publish jobs, providing high availability.
//
// When EnableLeaderElection is false, the server runs in single-instance mode,
// useful for embedded scenarios where you want direct control.
//
// # Micro Service Integration
//
// When EnableMicroService is true (default), the server registers NATS Micro
// service endpoints for job management. This provides:
//
//   - Service discovery via `nats micro list`
//   - Load-balanced API requests
//   - Standard NATS request-reply patterns
//
// When disabled, you interact with jobs directly via the scheduler instance.
//
// # Graceful Shutdown
//
// The server supports graceful shutdown with proper cleanup:
//
//	ctx, cancel := context.WithCancel(context.Background())
//
//	// Handle shutdown signals
//	c := make(chan os.Signal, 1)
//	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
//	go func() {
//		<-c
//		cancel() // This will cause srv.Wait() to return
//	}()
//
//	srv.Start(ctx)
//	srv.Wait() // Block until context is cancelled
//	srv.Stop() // Cleanup resources
//
// # Environment Variables
//
// The server respects standard environment variables when using DefaultOptions():
//
//   - NATS_URL: NATS server URL
//   - LOG_LEVEL: Logging level (debug, info, warn, error)
//   - INSTANCE_ID: Unique instance identifier
//
// These can be overridden by setting the corresponding fields in Options.
package server