# NATS Cron Examples

This directory contains examples showing how to use NATS Cron in real-world scenarios.

## Directory Structure

```
examples/
├── use-cases/          # Job definition examples
├── workers/            # Sample worker services
└── deployment/         # Docker deployment examples
```

## How It Works

1. **NATS Cron** schedules jobs by publishing messages to NATS subjects
2. **Worker services** subscribe to these subjects and do the actual work
3. **Multiple workers** can subscribe to the same subject for load balancing

## Quick Start

### 1. Start the System

```bash
# Start NATS and NATS Cron with Docker
cd examples/deployment
docker-compose up -d

# Or run locally
nats-server -js &
./bin/nats-cron &
```

### 2. Create a Job

```bash
# Create a database cleanup job
./bin/nats-cron-cli create examples/use-cases/database-cleanup.json

# List active jobs
./bin/nats-cron-cli list
```

### 3. Run a Worker

```bash
# Build and run the example worker
cd examples/workers
go run simple-worker.go
```

### 4. Watch It Work

The worker will receive and process messages sent by the scheduler according to the job's schedule.

## Example Jobs

### Database Cleanup (Daily at 2 AM)
```json
{
  "id": "db-cleanup-sessions",
  "target": { "subject": "database.cleanup" },
  "payload": {
    "type": "json",
    "data": "{\"table\":\"user_sessions\",\"older_than\":\"7d\"}"
  },
  "schedule": { "cron": "0 2 * * *" }
}
```

### Health Monitoring (Every 30 seconds)
```json
{
  "id": "health-check-api",
  "target": { "subject": "monitoring.health_check" },
  "payload": {
    "type": "json",
    "data": "{\"service\":\"user-api\",\"endpoint\":\"https://api.example.com/health\"}"
  },
  "schedule": { "every": "30s" }
}
```

### Report Generation (Weekly on Monday at 9 AM)
```json
{
  "id": "weekly-sales-report",
  "target": { "subject": "reports.generate" },
  "payload": {
    "type": "json",
    "data": "{\"type\":\"sales\",\"period\":\"weekly\",\"recipients\":[\"sales@company.com\"]}"
  },
  "schedule": { "cron": "0 9 * * MON" }
}
```

## Worker Implementation

Workers are simple NATS subscribers that process job messages:

```go
func main() {
    nc, _ := nats.Connect(nats.DefaultURL)
    
    nc.Subscribe("database.cleanup", func(msg *nats.Msg) {
        var jobData map[string]interface{}
        json.Unmarshal(msg.Data, &jobData)
        
        // Do the actual cleanup work here...
        log.Printf("Cleaning up table: %s", jobData["table"])
    })
    
    select {} // Keep running
}
```

## Architecture Benefits

- **Decoupled**: Scheduler and workers are independent
- **Scalable**: Multiple workers can process jobs in parallel
- **Language Agnostic**: Workers can be written in any language with NATS support
- **Reliable**: Jobs are persistent and survive restarts