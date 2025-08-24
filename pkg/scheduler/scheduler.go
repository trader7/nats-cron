package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/nats-io/nats.go"
	"github.com/oklog/ulid/v2"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type Scheduler struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	kv     nats.KeyValue
	gosch  *gocron.Scheduler
	crsch  *cron.Cron
	jobs   map[string]interface{}
	logger *zap.Logger
}

func New(nc *nats.Conn, js nats.JetStreamContext, kv nats.KeyValue, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		nc:     nc,
		js:     js,
		kv:     kv,
		gosch:  gocron.NewScheduler(time.UTC),
		crsch:  cron.New(cron.WithLocation(time.UTC)),
		jobs:   make(map[string]interface{}),
		logger: logger,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	// Start schedulers
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
		// If no keys found, that's fine - just means no jobs exist yet
		if err.Error() == "nats: no keys found" {
			s.logger.Info("No existing jobs found in KV store")
			return nil
		}
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
	watch, err := s.kv.Watch(">")
	if err != nil {
		return err
	}

	go func() {
		for update := range watch.Updates() {
			if update == nil {
				continue
			}

			// Handle deletions (check operation type)
			if update.Operation() == nats.KeyValueDelete || update.Operation() == nats.KeyValuePurge {
				subject := update.Key()

				// Remove from active jobs if running
				if existing, found := s.jobs[subject]; found {
					s.logger.Info("Job deleted, removing from scheduler", zap.String("subject", subject))
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

	if job.Subject == "" {
		s.logger.Error("Job has empty subject, skipping")
		return
	}

	if existing, found := s.jobs[job.Subject]; found {
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
		s.logger.Info("Executing job", zap.String("subject", jobCopy.Subject))
		now := time.Now()

		// Generate a ULID with current timestamp
		ulidValue := ulid.MustNew(ulid.Timestamp(now), rand.Reader)
		payloadData := []byte(ulidValue.String())
		s.logger.Debug("Generated ULID for job", zap.String("subject", jobCopy.Subject), zap.String("ulid", ulidValue.String()))

		err := s.nc.Publish(jobCopy.Subject, payloadData)
		if err != nil {
			s.logger.Error("Publish failed", zap.String("subject", jobCopy.Subject), zap.Error(err))
		} else {
			s.logger.Info("Published job", zap.String("subject", jobCopy.Subject))
		}

	}

	if job.Schedule.Every != "" {
		dur, err := time.ParseDuration(job.Schedule.Every)
		if err != nil {
			s.logger.Error("Invalid duration", zap.String("subject", job.Subject), zap.Error(err))
			return
		}
		j, err := s.gosch.Every(dur).Do(run)
		if err != nil {
			s.logger.Error("Failed to schedule job", zap.String("subject", job.Subject), zap.Error(err))
			return
		}
		s.jobs[job.Subject] = j
		s.logger.Info("Scheduled job", zap.String("subject", job.Subject), zap.String("interval", job.Schedule.Every))
		s.logger.Debug("Gocron job details", zap.String("subject", job.Subject), zap.Any("gocron_job", j), zap.Int("scheduler_job_count", len(s.gosch.Jobs())))
	} else if job.Schedule.Cron != "" {
		id, err := s.crsch.AddFunc(job.Schedule.Cron, run)
		if err != nil {
			s.logger.Error("Failed to schedule cron job", zap.String("subject", job.Subject), zap.Error(err))
			return
		}
		s.jobs[job.Subject] = id
		s.logger.Info("Scheduled cron job", zap.String("subject", job.Subject), zap.String("cron", job.Schedule.Cron))
	} else {
		s.logger.Error("Job has no schedule", zap.String("subject", job.Subject))
	}
}

func (s *Scheduler) GetActiveJobs() map[string]interface{} {
	return s.jobs
}
