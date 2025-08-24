package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/robfig/cron/v3"
	"github.com/trader7/nats-cron/internal/election"
	"github.com/trader7/nats-cron/pkg/config"
	"github.com/trader7/nats-cron/pkg/scheduler"
	"go.uber.org/zap"
)

// Server represents an embeddable NATS Cron server instance
type Server struct {
	options   *Options
	nc        *nats.Conn
	js        nats.JetStreamContext
	kv        nats.KeyValue
	scheduler *scheduler.Scheduler // nil when not leader
	election  *election.Manager
	service   micro.Service
	logger    *zap.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	mu        sync.RWMutex
	ownedConn bool // true if we created the NATS connection
}

// Options configures the NATS Cron server
type Options struct {
	// NATS connection (if nil, will create new connection using NATSURL)
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

// DefaultOptions returns a new Options struct with sensible defaults
func DefaultOptions() *Options {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "nats-cron"
	}

	return &Options{
		NATSURL:              nats.DefaultURL,
		ServiceName:          "nats-cron",
		ServiceVersion:       "1.0.0",
		InstanceID:           hostname,
		JobsBucketName:       "nats-cron-jobs",
		ElectionBucketName:   "nats-cron-leader",
		LogLevel:             "info",
		EnableMicroService:   true,
		EnableLeaderElection: true,
		ElectionTTL:          30 * time.Second,
		ShutdownTimeout:      30 * time.Second,
	}
}

// New creates a new NATS Cron server with the given options
func New(opts *Options) (*Server, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Apply environment variable defaults if not set
	if opts.NATSURL == nats.DefaultURL && os.Getenv("NATS_URL") != "" {
		opts.NATSURL = os.Getenv("NATS_URL")
	}
	if opts.LogLevel == "info" && os.Getenv("LOG_LEVEL") != "" {
		opts.LogLevel = os.Getenv("LOG_LEVEL")
	}
	if opts.InstanceID == "" || (opts.InstanceID != "" && os.Getenv("INSTANCE_ID") != "") {
		if instanceID := os.Getenv("INSTANCE_ID"); instanceID != "" {
			opts.InstanceID = instanceID
		}
	}

	server := &Server{
		options: opts,
	}

	// Setup logger
	if opts.Logger != nil {
		server.logger = opts.Logger
	} else {
		logger, err := config.NewLogger(opts.LogLevel)
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}
		server.logger = logger
	}

	// Setup NATS connection
	if opts.NATSConn != nil {
		server.nc = opts.NATSConn
		server.ownedConn = false
	} else {
		nc, err := nats.Connect(opts.NATSURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS: %w", err)
		}
		server.nc = nc
		server.ownedConn = true
	}

	// Setup JetStream
	js, err := server.nc.JetStream()
	if err != nil {
		if server.ownedConn {
			server.nc.Close()
		}
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}
	server.js = js

	// Create job storage bucket
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: opts.JobsBucketName,
	})
	if err != nil {
		kv, err = js.KeyValue(opts.JobsBucketName)
		if err != nil {
			if server.ownedConn {
				server.nc.Close()
			}
			return nil, fmt.Errorf("failed to access jobs bucket: %w", err)
		}
	}
	server.kv = kv

	// Scheduler will be created when becoming leader

	// Create election manager if enabled
	if opts.EnableLeaderElection {
		server.election = election.New(server.js, opts.InstanceID, opts.ElectionBucketName, server.logger)
	}

	return server, nil
}

// Start starts the NATS Cron server
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server is already running")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Create micro service if enabled
	if s.options.EnableMicroService {
		service, err := micro.AddService(s.nc, micro.Config{
			Name:        s.options.ServiceName,
			Version:     s.options.ServiceVersion,
			Description: "NATS Cron Scheduler",
			Metadata: map[string]string{
				"instance_id": s.options.InstanceID,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create micro service: %w", err)
		}
		s.service = service

		// Add service endpoints
		if err := s.addServiceEndpoints(); err != nil {
			return fmt.Errorf("failed to add service endpoints: %w", err)
		}
	}

	s.logger.Info("NATS Cron server starting",
		zap.String("instance_id", s.options.InstanceID),
		zap.String("nats_url", s.options.NATSURL),
		zap.Bool("micro_service", s.options.EnableMicroService),
		zap.Bool("leader_election", s.options.EnableLeaderElection))

	// Start leader election if enabled
	if s.options.EnableLeaderElection && s.election != nil {
		go s.election.Start(s.ctx, func(leaderCtx context.Context) {
			s.logger.Info("Elected as leader - creating and starting scheduler")
			
			// Create scheduler when becoming leader
			s.mu.Lock()
			s.scheduler = scheduler.New(s.nc, s.js, s.kv, s.logger)
			s.mu.Unlock()
			
			// Start scheduler
			if err := s.scheduler.Run(leaderCtx); err != nil {
				s.logger.Error("Scheduler error", zap.Error(err))
			}
			
			// Clean up scheduler when losing leadership
			s.mu.Lock()
			s.scheduler = nil
			s.mu.Unlock()
			s.logger.Info("Lost leadership - scheduler cleaned up")
		})
	} else {
		// If no leader election, create and start scheduler directly
		go func() {
			s.logger.Info("No leader election - creating and starting scheduler")
			
			s.mu.Lock()
			s.scheduler = scheduler.New(s.nc, s.js, s.kv, s.logger)
			s.mu.Unlock()
			
			if err := s.scheduler.Run(s.ctx); err != nil {
				s.logger.Error("Scheduler error", zap.Error(err))
			}
		}()
	}

	s.running = true
	s.logger.Info("NATS Cron server started")

	return nil
}

// Stop gracefully stops the NATS Cron server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping NATS Cron server...")

	// Cancel context to stop all goroutines
	if s.cancel != nil {
		s.cancel()
	}

	// Stop micro service
	if s.service != nil {
		if err := s.service.Stop(); err != nil {
			s.logger.Warn("Error stopping micro service", zap.Error(err))
		}
	}

	// Close NATS connection if we own it
	if s.ownedConn && s.nc != nil {
		s.nc.Close()
	}

	s.running = false
	s.logger.Info("NATS Cron server stopped")

	return nil
}

// IsRunning returns true if the server is currently running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// IsLeader returns true if this instance is currently the leader
// Returns false if leader election is disabled
func (s *Server) IsLeader() bool {
	if s.election == nil {
		return !s.options.EnableLeaderElection // true if election disabled
	}
	return s.election.IsLeader()
}

// GetScheduler returns the underlying scheduler instance for direct job management
// Returns nil if this instance is not the leader
func (s *Server) GetScheduler() *scheduler.Scheduler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scheduler
}

// Wait blocks until the server context is cancelled
func (s *Server) Wait() {
	if s.ctx != nil {
		<-s.ctx.Done()
	}
}

// Job management methods - these work on any instance by accessing KV directly

func (s *Server) GetJobs() ([]scheduler.JobStatus, error) {
	var statuses []scheduler.JobStatus
	keys, err := s.kv.Keys()
	if err != nil {
		// If no keys found, return empty list instead of error
		if err.Error() == "nats: no keys found" {
			return statuses, nil
		}
		return nil, err
	}

	for _, key := range keys {
		entry, err := s.kv.Get(key)
		if err != nil {
			continue
		}

		var job scheduler.JobDefinition
		if json.Unmarshal(entry.Value(), &job) == nil {
			statuses = append(statuses, scheduler.JobStatus{
				Subject: job.Subject,
			})
		}
	}
	return statuses, nil
}

func (s *Server) GetJob(subject string) (*scheduler.JobDefinition, error) {
	entry, err := s.kv.Get(subject)
	if err != nil {
		return nil, err
	}

	var job scheduler.JobDefinition
	if err := json.Unmarshal(entry.Value(), &job); err != nil {
		return nil, err
	}

	return &job, nil
}

func (s *Server) CreateJob(data []byte) error {
	var job scheduler.JobDefinition
	if err := json.Unmarshal(data, &job); err != nil {
		return err
	}

	if job.Subject == "" {
		return fmt.Errorf("job subject is required")
	}

	// Validate schedule
	if err := s.validateSchedule(job.Schedule); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}

	// Check if job already exists
	if _, err := s.kv.Get(job.Subject); err == nil {
		s.logger.Error("Duplicate job creation attempted", zap.String("subject", job.Subject))
		return fmt.Errorf("job with subject %s already exists", job.Subject)
	}

	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	_, err = s.kv.Create(job.Subject, jobData)
	if err != nil {
		return fmt.Errorf("failed to create job in KV store: %w", err)
	}
	return nil
}

func (s *Server) UpdateJob(data []byte) error {
	var job scheduler.JobDefinition
	if err := json.Unmarshal(data, &job); err != nil {
		return err
	}

	if job.Subject == "" {
		return fmt.Errorf("job subject is required")
	}

	// Validate schedule
	if err := s.validateSchedule(job.Schedule); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}

	if job.Subject == "" {
		return fmt.Errorf("job subject cannot be empty")
	}

	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	_, err = s.kv.Put(job.Subject, jobData)
	if err != nil {
		return fmt.Errorf("failed to update job in KV store: %w", err)
	}
	return nil
}

func (s *Server) DeleteJob(subject string) error {
	return s.kv.Delete(subject)
}

// DeleteJobsWithPattern deletes all jobs matching a NATS wildcard pattern
// Returns list of deleted job subjects and any error
func (s *Server) DeleteJobsWithPattern(pattern string) ([]string, error) {
	var deleted []string
	var lastError error

	// Get all job keys
	keys, err := s.kv.Keys()
	if err != nil {
		return nil, fmt.Errorf("failed to get job keys: %w", err)
	}

	// Find matching subjects
	var matches []string
	for _, key := range keys {
		if matchesNATSPattern(key, pattern) {
			matches = append(matches, key)
		}
	}

	// Delete each matching job
	for _, subject := range matches {
		err := s.DeleteJob(subject)
		if err != nil {
			s.logger.Error("Failed to delete job", zap.String("subject", subject), zap.Error(err))
			lastError = err
		} else {
			deleted = append(deleted, subject)
			s.logger.Info("Deleted job via pattern", zap.String("subject", subject), zap.String("pattern", pattern))
		}
	}

	return deleted, lastError
}

// validateSchedule validates job schedule configuration
func (s *Server) validateSchedule(schedule struct {
	Every string `json:"every,omitempty"`
	Cron  string `json:"cron,omitempty"`
}) error {
	// Must have exactly one schedule type
	if schedule.Every == "" && schedule.Cron == "" {
		return fmt.Errorf("schedule must specify either 'every' or 'cron'")
	}
	if schedule.Every != "" && schedule.Cron != "" {
		return fmt.Errorf("schedule cannot specify both 'every' and 'cron'")
	}

	// Validate duration format
	if schedule.Every != "" {
		_, err := time.ParseDuration(schedule.Every)
		if err != nil {
			return fmt.Errorf("invalid duration '%s': %w", schedule.Every, err)
		}
	}

	// Validate cron format
	if schedule.Cron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		_, err := parser.Parse(schedule.Cron)
		if err != nil {
			return fmt.Errorf("invalid cron expression '%s': %w", schedule.Cron, err)
		}
	}

	return nil
}

// matchesNATSPattern checks if a subject matches a NATS wildcard pattern
// Supports NATS wildcards:
// - * matches exactly one token (segment between dots)
// - > matches one or more trailing tokens
func matchesNATSPattern(subject, pattern string) bool {
	// Exact match case
	if subject == pattern {
		return true
	}

	// No wildcards, must be exact match
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, ">") {
		return false
	}

	subjectTokens := strings.Split(subject, ".")
	patternTokens := strings.Split(pattern, ".")

	return matchTokens(subjectTokens, patternTokens)
}

func matchTokens(subject, pattern []string) bool {
	si, pi := 0, 0

	for pi < len(pattern) && si < len(subject) {
		switch pattern[pi] {
		case "*":
			// * matches exactly one token
			si++
			pi++
		case ">":
			// > matches remaining tokens, must be last in pattern
			return pi == len(pattern)-1
		default:
			// Literal token must match exactly
			if subject[si] != pattern[pi] {
				return false
			}
			si++
			pi++
		}
	}

	// Check if we consumed all tokens correctly
	if pi < len(pattern) {
		// Remaining pattern tokens
		if len(pattern)-pi == 1 && pattern[pi] == ">" {
			// Pattern ends with >, matches any remaining subject tokens
			return true
		}
		// Unmatched pattern tokens (not ending with >)
		return false
	}

	// All pattern tokens consumed, subject should also be fully consumed
	return si == len(subject)
}

// addServiceEndpoints adds the NATS micro service endpoints
func (s *Server) addServiceEndpoints() error {
	// Job management endpoints
	if err := s.service.AddEndpoint("jobs-list", micro.HandlerFunc(func(req micro.Request) {
		jobs, err := s.GetJobs()
		if err != nil {
			req.Error("500", "Failed to get jobs", []byte(err.Error()))
			return
		}
		req.RespondJSON(map[string]interface{}{"jobs": jobs})
	}), micro.WithEndpointSubject("nats-cron.jobs.list")); err != nil {
		return fmt.Errorf("failed to add jobs.list endpoint: %w", err)
	}

	if err := s.service.AddEndpoint("jobs-create", micro.HandlerFunc(func(req micro.Request) {
		err := s.CreateJob(req.Data())
		if err != nil {
			req.RespondJSON(map[string]string{"error": err.Error()})
			return
		}

		req.RespondJSON(map[string]string{"status": "created"})
	}), micro.WithEndpointSubject("nats-cron.jobs.create")); err != nil {
		return fmt.Errorf("failed to add jobs.create endpoint: %w", err)
	}

	if err := s.service.AddEndpoint("jobs-update", micro.HandlerFunc(func(req micro.Request) {
		err := s.UpdateJob(req.Data())
		if err != nil {
			req.Error("400", "Failed to update job", []byte(err.Error()))
			return
		}
		req.RespondJSON(map[string]string{"status": "updated"})
	}), micro.WithEndpointSubject("nats-cron.jobs.update")); err != nil {
		return fmt.Errorf("failed to add jobs.update endpoint: %w", err)
	}

	if err := s.service.AddEndpoint("jobs-delete", micro.HandlerFunc(func(req micro.Request) {
		var payload struct {
			Subject string `json:"subject"`
		}
		if err := json.Unmarshal(req.Data(), &payload); err != nil {
			req.Error("400", "Invalid request", []byte(err.Error()))
			return
		}

		err := s.DeleteJob(payload.Subject)
		if err != nil {
			req.Error("400", "Failed to delete job", []byte(err.Error()))
			return
		}
		req.RespondJSON(map[string]string{"status": "deleted"})
	}), micro.WithEndpointSubject("nats-cron.jobs.delete")); err != nil {
		return fmt.Errorf("failed to add jobs.delete endpoint: %w", err)
	}

	if err := s.service.AddEndpoint("jobs-delete-pattern", micro.HandlerFunc(func(req micro.Request) {
		var payload struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(req.Data(), &payload); err != nil {
			req.Error("400", "Invalid request", []byte(err.Error()))
			return
		}

		deleted, err := s.DeleteJobsWithPattern(payload.Pattern)
		if err != nil {
			req.Error("400", "Failed to delete jobs", []byte(err.Error()))
			return
		}

		req.RespondJSON(map[string]interface{}{
			"deleted": deleted,
			"count":   len(deleted),
		})
	}), micro.WithEndpointSubject("nats-cron.jobs.delete.pattern")); err != nil {
		return fmt.Errorf("failed to add jobs.delete.pattern endpoint: %w", err)
	}

	if err := s.service.AddEndpoint("jobs-get", micro.HandlerFunc(func(req micro.Request) {
		var payload struct {
			Subject string `json:"subject"`
		}
		if err := json.Unmarshal(req.Data(), &payload); err != nil {
			req.Error("400", "Invalid request", []byte(err.Error()))
			return
		}

		job, err := s.GetJob(payload.Subject)
		if err != nil {
			req.Error("404", "Job not found", []byte(err.Error()))
			return
		}
		req.RespondJSON(job)
	}), micro.WithEndpointSubject("nats-cron.jobs.get")); err != nil {
		return fmt.Errorf("failed to add jobs.get endpoint: %w", err)
	}

	// Service status endpoint
	if err := s.service.AddEndpoint("status", micro.HandlerFunc(func(req micro.Request) {
		jobCount := 0
		if s.IsLeader() {
			s.mu.RLock()
			if s.scheduler != nil {
				jobCount = len(s.scheduler.GetActiveJobs())
			}
			s.mu.RUnlock()
		} else {
			// For non-leaders, count jobs from KV store
			jobs, err := s.GetJobs()
			if err == nil {
				jobCount = len(jobs)
			}
		}
		status := map[string]interface{}{
			"service":   s.options.ServiceName,
			"is_leader": s.IsLeader(),
			"jobs":      jobCount,
		}
		req.RespondJSON(status)
	}), micro.WithEndpointSubject("nats-cron.status")); err != nil {
		return fmt.Errorf("failed to add status endpoint: %w", err)
	}

	return nil
}
