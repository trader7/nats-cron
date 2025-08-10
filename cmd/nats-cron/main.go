package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/trader7/nats-cron/internal/election"
	"github.com/trader7/nats-cron/pkg/config"
	"github.com/trader7/nats-cron/pkg/scheduler"
	"go.uber.org/zap"
)

func main() {
	// Handle help flag
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help") {
		printUsage()
		return
	}

	cfg := config.New()

	logger, err := config.NewLogger(cfg.LogLevel)
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logger.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		logger.Fatal("Failed to create JetStream context", zap.Error(err))
	}

	// Create job storage bucket
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: "scheduler_jobs",
	})
	if err != nil {
		kv, err = js.KeyValue("scheduler_jobs")
		if err != nil {
			logger.Fatal("Failed to access jobs bucket", zap.Error(err))
		}
	}

	// Create scheduler service
	sched := scheduler.New(nc, js, kv, logger)
	
	// Create election manager
	electionMgr := election.New(js, cfg.InstanceID, logger)

	// Create micro service
	service, err := micro.AddService(nc, micro.Config{
		Name:        "nats-cron",
		Version:     "1.0.0",
		Description: "NATS Cron Scheduler",
		Metadata: map[string]string{
			"instance_id": cfg.InstanceID,
		},
	})
	if err != nil {
		logger.Fatal("Failed to create micro service", zap.Error(err))
	}

	// Add service endpoints
	addServiceEndpoints(service, sched, electionMgr, logger)

	logger.Info("NATS Cron service started", 
		zap.String("instance_id", cfg.InstanceID),
		zap.String("nats_url", cfg.NATSURL))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.Info("Shutting down...")
		cancel()
	}()

	// Start election process (only leader will schedule jobs)
	go electionMgr.Start(ctx, func(leaderCtx context.Context) {
		logger.Info("Elected as leader - starting scheduler")
		sched.Run(leaderCtx)
	})

	// Wait for context cancellation
	<-ctx.Done()
	service.Stop()
	logger.Info("Service stopped")
}

func addServiceEndpoints(service micro.Service, sched *scheduler.Scheduler, election *election.Manager, logger *zap.Logger) {
	// Job management endpoints
	if err := service.AddEndpoint("jobs-list", micro.HandlerFunc(func(req micro.Request) {
		jobs, err := sched.GetJobs()
		if err != nil {
			req.Error("500", "Failed to get jobs", []byte(err.Error()))
			return
		}
		req.RespondJSON(map[string]interface{}{"jobs": jobs})
	}), micro.WithEndpointSubject("nats-cron.jobs.list")); err != nil {
		logger.Fatal("Failed to add jobs.list endpoint", zap.Error(err))
	}

	if err := service.AddEndpoint("jobs-create", micro.HandlerFunc(func(req micro.Request) {
		err := sched.CreateJob(req.Data())
		if err != nil {
			req.Error("400", "Failed to create job", []byte(err.Error()))
			return
		}
		
		// Manually trigger job scheduling since KV watch might not catch it
		sched.ScheduleJobFromData(req.Data())
		
		req.RespondJSON(map[string]string{"status": "created"})
	}), micro.WithEndpointSubject("nats-cron.jobs.create")); err != nil {
		logger.Fatal("Failed to add jobs.create endpoint", zap.Error(err))
	}

	if err := service.AddEndpoint("jobs-update", micro.HandlerFunc(func(req micro.Request) {
		err := sched.UpdateJob(req.Data())
		if err != nil {
			req.Error("400", "Failed to update job", []byte(err.Error()))
			return
		}
		req.RespondJSON(map[string]string{"status": "updated"})
	}), micro.WithEndpointSubject("nats-cron.jobs.update")); err != nil {
		logger.Fatal("Failed to add jobs.update endpoint", zap.Error(err))
	}

	if err := service.AddEndpoint("jobs-delete", micro.HandlerFunc(func(req micro.Request) {
		var payload struct {
			Subject string `json:"subject"`
		}
		if err := json.Unmarshal(req.Data(), &payload); err != nil {
			req.Error("400", "Invalid request", []byte(err.Error()))
			return
		}

		err := sched.DeleteJob(payload.Subject)
		if err != nil {
			req.Error("400", "Failed to delete job", []byte(err.Error()))
			return
		}
		req.RespondJSON(map[string]string{"status": "deleted"})
	}), micro.WithEndpointSubject("nats-cron.jobs.delete")); err != nil {
		logger.Fatal("Failed to add jobs.delete endpoint", zap.Error(err))
	}

	if err := service.AddEndpoint("jobs-delete-pattern", micro.HandlerFunc(func(req micro.Request) {
		var payload struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(req.Data(), &payload); err != nil {
			req.Error("400", "Invalid request", []byte(err.Error()))
			return
		}

		deleted, err := sched.DeleteJobsWithPattern(payload.Pattern)
		if err != nil {
			req.Error("400", "Failed to delete jobs", []byte(err.Error()))
			return
		}
		
		req.RespondJSON(map[string]interface{}{
			"deleted": deleted,
			"count":   len(deleted),
		})
	}), micro.WithEndpointSubject("nats-cron.jobs.delete.pattern")); err != nil {
		logger.Fatal("Failed to add jobs.delete.pattern endpoint", zap.Error(err))
	}

	if err := service.AddEndpoint("jobs-get", micro.HandlerFunc(func(req micro.Request) {
		var payload struct {
			Subject string `json:"subject"`
		}
		if err := json.Unmarshal(req.Data(), &payload); err != nil {
			req.Error("400", "Invalid request", []byte(err.Error()))
			return
		}

		job, err := sched.GetJob(payload.Subject)
		if err != nil {
			req.Error("404", "Job not found", []byte(err.Error()))
			return
		}
		req.RespondJSON(job)
	}), micro.WithEndpointSubject("nats-cron.jobs.get")); err != nil {
		logger.Fatal("Failed to add jobs.get endpoint", zap.Error(err))
	}

	// Service status endpoint
	if err := service.AddEndpoint("status", micro.HandlerFunc(func(req micro.Request) {
		status := map[string]interface{}{
			"service":   "nats-cron",
			"is_leader": election.IsLeader(),
			"jobs":      len(sched.GetActiveJobs()),
		}
		req.RespondJSON(status)
	}), micro.WithEndpointSubject("nats-cron.status")); err != nil {
		logger.Fatal("Failed to add status endpoint", zap.Error(err))
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
	fmt.Println("  Use 'nats-cron-cli' command to manage jobs")
	fmt.Println("  Use 'nats micro list' to see running instances")
}