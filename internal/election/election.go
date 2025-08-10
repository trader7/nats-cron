package election

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	election "github.com/ripienaar/nats-kv-leader-elect"
	"go.uber.org/zap"
)

type Manager struct {
	js       nats.JetStreamContext
	id       string
	logger   *zap.Logger
	elector  election.Election
	isLeader bool
}

func New(js nats.JetStreamContext, instanceID string, logger *zap.Logger) *Manager {
	return &Manager{
		js:     js,
		id:     instanceID,
		logger: logger,
	}
}

func (m *Manager) Start(ctx context.Context, onLeader func(context.Context)) {
	bucketName := "scheduler_leader"
	
	// Create election bucket with fast failover TTL (minimum allowed is 30s)
	kv, err := m.js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: bucketName,
		TTL:    30 * time.Second, // Minimum TTL for faster failover
	})
	if err != nil {
		// Try to get existing bucket if creation fails
		kv, err = m.js.KeyValue(bucketName)
		if err != nil {
			m.logger.Fatal("Failed to access election bucket", zap.Error(err))
		}
	}

	// Create elector
	elector, err := election.NewElection("nats-cron", "leader", kv)
	if err != nil {
		m.logger.Fatal("Failed to create elector", zap.Error(err))
	}
	m.elector = elector

	// Start election process with fast polling for responsive failover
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if elector.IsLeader() {
					if !m.isLeader {
						m.isLeader = true
						m.logger.Info("Elected as leader")
						go onLeader(ctx)
					}
				} else {
					if m.isLeader {
						m.isLeader = false
						m.logger.Info("Lost leadership")
					}
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()

	elector.Start(ctx)
}

func (m *Manager) IsLeader() bool {
	return m.isLeader
}