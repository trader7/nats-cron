# Embedding NATS Cron in Your Go Application

This directory contains examples showing how to embed the NATS Cron scheduler directly into your Go applications.

## Quick Start

The simplest way to embed NATS Cron:

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    
    "github.com/trader7/nats-cron/pkg/server"
)

func main() {
    // Create server with default options
    srv, err := server.New(server.DefaultOptions())
    if err != nil {
        log.Fatal(err)
    }

    // Start the server
    ctx := context.Background()
    if err := srv.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer srv.Stop()

    // Create a scheduled job
    jobData, _ := json.Marshal(map[string]interface{}{
        "target": map[string]string{"subject": "my.app.task"},
        "payload": map[string]string{"data": "Hello World"},
        "schedule": map[string]string{"every": "30s"},
    })
    
    srv.GetScheduler().CreateJob(jobData)
    
    // Your app logic here...
    srv.Wait() // Block until shutdown
}
```

## Configuration Options

### Server Options

```go
type Options struct {
    // NATS connection (if nil, creates new connection using NATSURL)
    NATSConn *nats.Conn
    
    // NATS server URL (used only if NATSConn is nil)
    NATSURL string
    
    // Service configuration
    ServiceName    string
    ServiceVersion string
    InstanceID     string
    
    // Storage configuration
    JobsBucketName     string
    ElectionBucketName string
    
    // Logging
    Logger   *zap.Logger
    LogLevel string
    
    // Service registration
    EnableMicroService bool
    
    // Election configuration
    EnableLeaderElection bool
    ElectionTTL          time.Duration
    
    // Graceful shutdown timeout
    ShutdownTimeout time.Duration
}
```

### Default Values

```go
opts := server.DefaultOptions()
// NATSURL: "nats://localhost:4222"
// ServiceName: "nats-cron"
// ServiceVersion: "1.0.0"
// InstanceID: hostname
// JobsBucketName: "scheduler_jobs"
// ElectionBucketName: "scheduler_leader"
// LogLevel: "info"
// EnableMicroService: true
// EnableLeaderElection: true
// ElectionTTL: 30 * time.Second
// ShutdownTimeout: 30 * time.Second
```

## Usage Patterns

### 1. Basic Embedding

Use default configuration with environment variable support:

```go
srv, _ := server.New(server.DefaultOptions())
srv.Start(context.Background())
defer srv.Stop()
```

### 2. Custom Configuration

Override specific settings:

```go
opts := server.DefaultOptions()
opts.ServiceName = "my-app-scheduler"
opts.JobsBucketName = "my_app_jobs"
opts.EnableMicroService = false

srv, _ := server.New(opts)
```

### 3. Reuse Existing NATS Connection

Perfect for apps already using NATS:

```go
nc, _ := nats.Connect("nats://localhost:4222")

opts := server.DefaultOptions()
opts.NATSConn = nc // Reuse connection

srv, _ := server.New(opts)
```

### 4. Embedded Mode (No Micro Service)

For internal scheduling without exposing APIs:

```go
opts := server.DefaultOptions()
opts.EnableMicroService = false

srv, _ := server.New(opts)
```

### 5. Custom Logger Integration

Use your app's existing logger:

```go
logger, _ := zap.NewProduction()

opts := server.DefaultOptions()
opts.Logger = logger

srv, _ := server.New(opts)
```

## Job Management

### Direct Scheduler Access

```go
sched := srv.GetScheduler()

// Create job
jobData, _ := json.Marshal(jobDefinition)
sched.CreateJob(jobData)

// List jobs
jobs, _ := sched.GetJobs()

// Get specific job
job, _ := sched.GetJob("my.subject")

// Update job
sched.UpdateJob(updatedJobData)

// Delete job
sched.DeleteJob("my.subject")

// Delete multiple jobs with pattern
deleted, _ := sched.DeleteJobsWithPattern("my.app.*")
```

### Job Definition Format

```go
jobDefinition := map[string]interface{}{
    "target": map[string]string{
        "subject": "my.app.task",
    },
    "payload": map[string]string{
        "type": "json",
        "data": `{"action": "cleanup", "table": "sessions"}`,
    },
    "schedule": map[string]string{
        "every": "1h",        // or
        "cron": "0 2 * * *",  // 2 AM daily
    },
}
```

## Server Lifecycle

### Starting and Stopping

```go
srv, _ := server.New(opts)

// Start (non-blocking)
ctx := context.Background()
srv.Start(ctx)

// Check status
fmt.Printf("Running: %v\n", srv.IsRunning())
fmt.Printf("Leader: %v\n", srv.IsLeader())

// Wait for shutdown
srv.Wait()

// Graceful stop
srv.Stop()
```

### Graceful Shutdown

```go
ctx, cancel := context.WithCancel(context.Background())

// Handle signals
c := make(chan os.Signal, 1)
signal.Notify(c, os.Interrupt, syscall.SIGTERM)
go func() {
    <-c
    cancel() // Triggers srv.Wait() to return
}()

srv.Start(ctx)
srv.Wait()
srv.Stop()
```

## Leader Election

When `EnableLeaderElection` is true (default):

- Multiple server instances can run
- Only the leader schedules and publishes jobs
- Automatic failover if leader goes down
- All instances can handle API requests (if micro service enabled)

When `EnableLeaderElection` is false:

- Single instance mode
- No coordination with other instances
- Useful for embedded scenarios

## Message Handling

Set up subscribers to handle scheduled messages:

```go
nc.Subscribe("my.app.cleanup", func(msg *nats.Msg) {
    var payload map[string]interface{}
    json.Unmarshal(msg.Data, &payload)
    
    // Handle the scheduled task
    handleCleanup(payload)
})
```

## Error Handling

```go
srv, err := server.New(opts)
if err != nil {
    log.Fatalf("Failed to create server: %v", err)
}

if err := srv.Start(ctx); err != nil {
    log.Fatalf("Failed to start server: %v", err)
}

if err := srv.Stop(); err != nil {
    log.Printf("Error during shutdown: %v", err)
}
```

## Examples

- `simple.go` - Minimal embedding example
- `main.go` - Comprehensive examples showing all patterns
- Run examples: `go run simple.go` or `go run main.go`

## Environment Variables

The embedded server respects these environment variables (when using DefaultOptions):

- `NATS_URL` - NATS server URL
- `LOG_LEVEL` - Logging level (debug, info, warn, error)
- `INSTANCE_ID` - Unique instance identifier

## Best Practices

1. **Reuse NATS connections** when possible to avoid connection overhead
2. **Use custom bucket names** to avoid conflicts with other applications
3. **Disable micro service** if you don't need the API endpoints
4. **Set custom instance IDs** in distributed deployments
5. **Handle graceful shutdown** properly to avoid job interruption
6. **Subscribe to job subjects** before starting the scheduler
7. **Use structured logging** for better observability