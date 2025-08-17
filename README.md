# NATS Cron

A distributed, highly available cron scheduler built on NATS JetStream with leader election.

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

## Features

- **Distributed**: Multiple instances with automatic leader election
- **High Availability**: Leader failover with no job loss
- **Embeddable**: Can be embedded directly into Go applications as a library
- **NATS Micro**: Built on NATS Micro framework for observability and load balancing
- **Flexible Scheduling**: Supports both interval-based (`every: 30s`) and cron-based (`cron: 0 9 * * *`) scheduling
- **Simple CLI**: Easy-to-use command-line interface for job management
- **Structured Logging**: JSON-formatted logs with configurable levels
- **Job Persistence**: Jobs survive restarts and leader changes

## How It Works

NATS Cron is a **message scheduler** that publishes messages to NATS subjects on a schedule. It doesn't execute jobs directly - instead, it triggers other services that subscribe to these messages.

```
┌─────────────────┐    schedule    ┌─────────────────┐    message    ┌─────────────────┐
│   NATS Cron     │───────────────→│   NATS Server   │──────────────→│  Worker Services│
│   Scheduler     │                │                 │               │                 │
└─────────────────┘                └─────────────────┘               └─────────────────┘
```

This creates a **decoupled, event-driven architecture** where:
- **NATS Micro service** handles API requests (any instance can respond)
- **Leader election** ensures only one instance schedules jobs
- **Workers** handle the actual business logic  
- **Load balancing** distributes work across multiple workers
- **Language agnostic** - workers can be written in any language with NATS support

## Quick Start

### 1. Start the System

```bash
# Clone and build
git clone https://github.com/trader7/nats-cron.git
cd nats-cron
make build

# Start NATS with JetStream (download nats-server from https://github.com/nats-io/nats-server/releases)
nats-server -js &

# Start NATS Cron scheduler
./bin/nats-cron &
```

### 2. Create a Worker Service

```go
// worker.go
package main

import (
    "encoding/json"
    "log"
    "github.com/nats-io/nats.go"
)

func main() {
    nc, _ := nats.Connect(nats.DefaultURL)
    defer nc.Close()

    // Subscribe to cleanup jobs
    nc.Subscribe("database.cleanup", func(msg *nats.Msg) {
        var job map[string]interface{}
        json.Unmarshal(msg.Data, &job)
        
        log.Printf("Cleaning up table: %s", job["table"])
        // Do actual cleanup work here...
    })

    select {} // Keep running
}
```

### 3. Schedule a Job

```bash
# Create job definition
cat > cleanup-job.json << EOF
{
  "target": {
    "subject": "database.cleanup"
  },
  "payload": {
    "type": "json",
    "data": "{\"table\":\"user_sessions\",\"older_than\":\"7d\"}"
  },
  "schedule": {
    "cron": "0 2 * * *"
  }
}
EOF

# Schedule the job
./bin/nats-cron-cli create cleanup-job.json

# List active jobs
./bin/nats-cron-cli list

# Check service status (shows which instance is leader)
./bin/nats-cron-cli status
```

### 4. Run Your Worker

```bash
go run worker.go
```

Now every day at 2 AM, NATS Cron will publish a message to `database.cleanup`, and your worker will receive it and perform the cleanup.

### 5. Or Use the Simple CLI (Alternative to JSON files)

For quick job creation without writing JSON files:

```bash
# Add a simple interval job
./bin/nats-cron-cli add system.heartbeat 30s

# Add a job with JSON payload  
./bin/nats-cron-cli add database.cleanup 1h '{"table":"sessions","older_than":"7d"}'

# Add a cron-based job
./bin/nats-cron-cli add reports.daily "0 9 * * *"

# List all jobs
./bin/nats-cron-cli list
```

**Note:** The CLI now uses the subject as the primary identifier - no more separate IDs to manage!

## Embedding in Go Applications

NATS Cron can be embedded directly into your Go applications as a library:

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    
    "github.com/trader7/nats-cron/pkg/server"
)

func main() {
    // Create and start embedded scheduler
    srv, err := server.New(server.DefaultOptions())
    if err != nil {
        log.Fatal(err)
    }
    
    ctx := context.Background()
    srv.Start(ctx)
    defer srv.Stop()
    
    // Create a scheduled job
    jobData, _ := json.Marshal(map[string]interface{}{
        "target": map[string]string{"subject": "my.app.task"},
        "payload": map[string]string{"data": "Hello World"},
        "schedule": map[string]string{"every": "30s"},
    })
    
    srv.GetScheduler().CreateJob(jobData)
    
    // Your app logic here...
    srv.Wait()
}
```

### Embedding Benefits

- **No separate process**: Scheduler runs inside your application
- **Shared NATS connection**: Reuse existing connections
- **Custom configuration**: Full control over settings
- **Direct API access**: No need for CLI or HTTP calls
- **Graceful integration**: Follows your app's lifecycle

See [examples/embedded/](examples/embedded/) for comprehensive embedding examples.

## NATS Micro Benefits

With NATS Micro framework integration:

- **Service Discovery**: `nats micro list` shows all running instances
- **Load Balancing**: Any instance can handle API requests  
- **Observability**: Built-in metrics and monitoring
- **Leader Status**: Easy to see which instance is currently scheduling jobs

## Job Configuration

Jobs are defined using JSON with the following structure:

```json
{
  "target": {
    "subject": "my.subject"
  },
  "payload": {
    "type": "json",
    "data": "{\"message\":\"hello world\"}"
  },
  "schedule": {
    "every": "30s"
  }
}
```

### Schedule Types

#### Interval-based scheduling
```json
{
  "schedule": {
    "every": "30s"
  }
}
```

Supported intervals: `ns`, `us`, `ms`, `s`, `m`, `h`

#### Cron-based scheduling
```json
{
  "schedule": {
    "cron": "0 9 * * *"
  }
}
```

Standard cron format: `minute hour day month weekday`

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `INSTANCE_ID` | system hostname | Unique instance identifier |

## Use Cases

### Database Maintenance
```json
{
  "target": { "subject": "database.cleanup" },
  "payload": { "data": "{\"table\":\"sessions\",\"older_than\":\"7d\"}" },
  "schedule": { "cron": "0 2 * * *" }
}
```

### Health Monitoring
```json
{
  "target": { "subject": "monitoring.health_check" },
  "payload": { "data": "{\"service\":\"api\",\"endpoint\":\"https://api.example.com/health\"}" },
  "schedule": { "every": "30s" }
}
```

### Report Generation
```json
{
  "target": { "subject": "reports.generate" },
  "payload": { "data": "{\"type\":\"sales\",\"recipients\":[\"team@company.com\"]}" },
  "schedule": { "cron": "0 9 * * MON" }
}
```

### Cache Warming
```json
{
  "target": { "subject": "cache.warm" },
  "payload": { "data": "{\"keys\":[\"popular_products\",\"categories\"]}" },
  "schedule": { "every": "15m" }
}
```

## Architecture

NATS Cron uses **NATS Micro** for service discovery and load balancing, with **leader election** for job scheduling:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ NATS Cron (1)   │    │ NATS Cron (2)   │    │ NATS Cron (3)   │
│ [Leader+API]    │    │ [Candidate+API] │    │ [Candidate+API] │
│ • Schedules     │    │ • API only      │    │ • API only      │
│ • Handles APIs  │    │ • Load balanced │    │ • Load balanced │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────────┐
                    │   NATS JetStream    │
         ┌──────────┤                     ├──────────┐
         │          │ • Job Storage       │          │  
         │          │ • Leader Election   │          │
         │          │ • Micro Discovery   │          │
         │          └─────────────────────┘          │
         │                                           │
┌─────────────────┐                         ┌─────────────────┐
│ Worker Pool A   │                         │ Worker Pool B   │
│ (Cleanup Jobs)  │                         │ (Report Jobs)   │
└─────────────────┘                         └─────────────────┘
```

**Key Benefits:**
- Any instance can handle API requests (load balanced)
- Only the leader schedules and publishes job messages
- Service discovery with `nats micro list`
- Built-in observability and health checks

## Development

### Build

```bash
# Build both server and CLI
make build

# Build server only
make build-server

# Build CLI only
make build-cli
```

### Testing

```bash
# Run tests
make test

# Format code
make fmt

# Run linter (requires golangci-lint)
make lint
```

### Docker

```bash
# Build Docker image
make docker

# Run with Docker
docker run -e NATS_URL=nats://host.docker.internal:4222 nats-cron:latest
```

## API

The scheduler exposes NATS request-reply endpoints:

- `nats-cron.jobs.list` - Get all job statuses  
- `nats-cron.jobs.get` - Get specific job details (by subject)
- `nats-cron.jobs.create` - Create a new job
- `nats-cron.jobs.update` - Update an existing job  
- `nats-cron.jobs.delete` - Delete a job (by subject)
- `nats-cron.status` - Get service status

**Note:** Jobs are now identified by their subject rather than a separate ID field. This simplifies the API and makes it more intuitive.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run `make test fmt vet`
6. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.