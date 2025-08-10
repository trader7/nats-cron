package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/nats-io/nats.go"
	"github.com/oklog/ulid/v2"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type Scheduler struct {
	nc    *nats.Conn
	js    nats.JetStreamContext
	kv    nats.KeyValue
	gosch *gocron.Scheduler
	crsch *cron.Cron
	jobs  map[string]interface{}
	logger *zap.Logger
}

func New(nc *nats.Conn, js nats.JetStreamContext, kv nats.KeyValue, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		nc:    nc,
		js:    js,
		kv:    kv,
		gosch: gocron.NewScheduler(time.UTC),
		crsch: cron.New(cron.WithLocation(time.UTC)),
		jobs:  make(map[string]interface{}),
		logger: logger,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	// Start schedulers FIRST, before loading jobs
	s.logger.Info("Starting gocron scheduler")
	s.gosch.StartAsync()
	s.logger.Info("Starting cron scheduler")
	s.crsch.Start()
	s.logger.Info("Schedulers started")

	if err := s.loadExistingJobs(); err != nil {
		return err
	}
	
	if err := s.watchForJobChanges(); err != nil {
		return err
	}
	
	s.logger.Info("Job loading and watching complete", zap.Int("gocron_jobs", len(s.gosch.Jobs())))

	<-ctx.Done()
	s.gosch.Clear()
	s.crsch.Stop()
	return nil
}

func (s *Scheduler) loadExistingJobs() error {
	keys, err := s.kv.Keys()
	if err != nil {
		s.logger.Error("Error loading KV keys", zap.Error(err))
		return err
	}
	
	for _, key := range keys {
		entry, err := s.kv.Get(key)
		if err == nil {
			s.scheduleJob(entry.Value())
		}
	}
	return nil
}

func (s *Scheduler) watchForJobChanges() error {
	watch, err := s.kv.Watch("")
	if err != nil {
		return err
	}

	go func() {
		for update := range watch.Updates() {
			if update == nil {
				continue
			}
			
			// Handle deletions (when Value() is nil)
			if update.Value() == nil {
				subject := update.Key()
				s.logger.Info("Job deleted, removing from scheduler", zap.String("subject", subject))
				
				// Remove from active jobs if running
				if existing, found := s.jobs[subject]; found {
					switch e := existing.(type) {
					case *gocron.Job:
						s.gosch.RemoveByReference(e)
					case cron.EntryID:
						s.crsch.Remove(e)
					}
					delete(s.jobs, subject)
				}
				continue
			}
			
			// Handle additions/updates
			s.scheduleJob(update.Value())
		}
	}()
	
	return nil
}

func (s *Scheduler) ScheduleJobFromData(data []byte) {
	s.scheduleJob(data)
}

func (s *Scheduler) scheduleJob(data []byte) {
	var job JobDefinition
	if err := json.Unmarshal(data, &job); err != nil {
		s.logger.Error("Invalid job", zap.Error(err))
		return
	}

	if existing, found := s.jobs[job.Target.Subject]; found {
		switch e := existing.(type) {
		case *gocron.Job:
			s.gosch.RemoveByReference(e)
		case cron.EntryID:
			s.crsch.Remove(e)
		}
	}

	// Capture job by value to avoid closure issues
	jobCopy := job
	run := func() {
		s.logger.Info("Executing job", zap.String("subject", jobCopy.Target.Subject))
		now := time.Now()
		
		// Generate payload data - use ULID if no data specified
		var payloadData []byte
		if jobCopy.Payload.Data == "" {
			// Generate a ULID with current timestamp
			ulidValue := ulid.MustNew(ulid.Timestamp(now), rand.Reader)
			payloadData = []byte(ulidValue.String())
			s.logger.Debug("Generated ULID for empty payload", zap.String("subject", jobCopy.Target.Subject), zap.String("ulid", ulidValue.String()))
		} else {
			payloadData = []byte(jobCopy.Payload.Data)
		}
		
		err := s.nc.Publish(jobCopy.Target.Subject, payloadData)
		if err != nil {
			s.logger.Error("Publish failed", zap.String("subject", jobCopy.Target.Subject), zap.Error(err))
		} else {
			s.logger.Info("Published job", zap.String("subject", jobCopy.Target.Subject))
		}

		jobCopy.LastRun = now
		if jobCopy.Schedule.Every != "" {
			dur, _ := time.ParseDuration(jobCopy.Schedule.Every)
			jobCopy.NextRun = now.Add(dur)
		} else if jobCopy.Schedule.Cron != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			schedule, _ := parser.Parse(jobCopy.Schedule.Cron)
			jobCopy.NextRun = schedule.Next(now)
		}
		s.saveJobState(jobCopy)
	}

	if job.Schedule.Every != "" {
		dur, err := time.ParseDuration(job.Schedule.Every)
		if err != nil {
			s.logger.Error("Invalid duration", zap.String("subject", job.Target.Subject), zap.Error(err))
			return
		}
		job.NextRun = time.Now().Add(dur)
		j, err := s.gosch.Every(dur).Do(run)
		if err != nil {
			s.logger.Error("Failed to schedule job", zap.String("subject", job.Target.Subject), zap.Error(err))
			return
		}
		s.jobs[job.Target.Subject] = j
		s.saveJobState(job)
		s.logger.Info("Scheduled job", zap.String("subject", job.Target.Subject), zap.String("interval", job.Schedule.Every))
		s.logger.Debug("Gocron job details", zap.String("subject", job.Target.Subject), zap.Any("gocron_job", j), zap.Int("scheduler_job_count", len(s.gosch.Jobs())))
	} else if job.Schedule.Cron != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := parser.Parse(job.Schedule.Cron)
		if err != nil {
			s.logger.Error("Invalid cron", zap.String("subject", job.Target.Subject), zap.Error(err))
			return
		}
		job.NextRun = schedule.Next(time.Now())
		id, err := s.crsch.AddFunc(job.Schedule.Cron, run)
		if err != nil {
			s.logger.Error("Failed to schedule cron job", zap.String("subject", job.Target.Subject), zap.Error(err))
			return
		}
		s.jobs[job.Target.Subject] = id
		s.saveJobState(job)
		s.logger.Info("Scheduled cron job", zap.String("subject", job.Target.Subject), zap.String("cron", job.Schedule.Cron))
	} else {
		s.logger.Error("Job has no schedule", zap.String("subject", job.Target.Subject))
	}
}

func (s *Scheduler) saveJobState(job JobDefinition) {
	data, err := json.Marshal(job)
	if err == nil {
		s.kv.Put(job.Target.Subject, data)
	}
}

func (s *Scheduler) GetJobs() ([]JobStatus, error) {
	var statuses []JobStatus
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
		
		var job JobDefinition
		if json.Unmarshal(entry.Value(), &job) == nil {
			statuses = append(statuses, JobStatus{
				Subject: job.Target.Subject,
				LastRun: job.LastRun,
				NextRun: job.NextRun,
			})
		}
	}
	return statuses, nil
}

func (s *Scheduler) GetJob(subject string) (*JobDefinition, error) {
	entry, err := s.kv.Get(subject)
	if err != nil {
		return nil, err
	}
	
	var job JobDefinition
	if err := json.Unmarshal(entry.Value(), &job); err != nil {
		return nil, err
	}
	
	return &job, nil
}

func (s *Scheduler) CreateJob(data []byte) error {
	var job JobDefinition
	if err := json.Unmarshal(data, &job); err != nil {
		return err
	}
	
	if job.Target.Subject == "" {
		return fmt.Errorf("job subject is required")
	}
	
	// Validate schedule
	if err := s.validateSchedule(job.Schedule); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	
	// Check if job already exists
	if _, err := s.kv.Get(job.Target.Subject); err == nil {
		return fmt.Errorf("job with subject %s already exists", job.Target.Subject)
	}
	
	jobData, _ := json.Marshal(job)
	_, err := s.kv.Create(job.Target.Subject, jobData)
	return err
}

func (s *Scheduler) UpdateJob(data []byte) error {
	var job JobDefinition
	if err := json.Unmarshal(data, &job); err != nil {
		return err
	}
	
	if job.Target.Subject == "" {
		return fmt.Errorf("job subject is required")
	}
	
	// Validate schedule
	if err := s.validateSchedule(job.Schedule); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	
	jobData, _ := json.Marshal(job)
	_, err := s.kv.Put(job.Target.Subject, jobData)
	return err
}

func (s *Scheduler) DeleteJob(subject string) error {
	// Remove from active jobs if running
	if existing, found := s.jobs[subject]; found {
		switch e := existing.(type) {
		case *gocron.Job:
			s.gosch.RemoveByReference(e)
		case cron.EntryID:
			s.crsch.Remove(e)
		}
		delete(s.jobs, subject)
	}
	
	return s.kv.Delete(subject)
}

// DeleteJobsWithPattern deletes all jobs matching a NATS wildcard pattern
// Returns list of deleted job subjects and any error
func (s *Scheduler) DeleteJobsWithPattern(pattern string) ([]string, error) {
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

func (s *Scheduler) GetActiveJobs() map[string]interface{} {
	return s.jobs
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

// validateSchedule validates job schedule configuration
func (s *Scheduler) validateSchedule(schedule struct {
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