module embedded-example

go 1.23.4

replace github.com/trader7/nats-cron => ../..

require (
	github.com/nats-io/nats.go v1.44.0
	github.com/trader7/nats-cron v0.0.0-00010101000000-000000000000
	go.uber.org/zap v1.27.0
)

require (
	github.com/go-co-op/gocron v1.37.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/oklog/ulid/v2 v2.1.0 // indirect
	github.com/ripienaar/nats-kv-leader-elect v0.0.0-20211103102849-959bf24a85de // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
)
