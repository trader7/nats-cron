# nats-cron
nats-cron is a distributed, leader-elected job scheduler that publishes messages to NATS subjects on a fixed interval or cron schedule. Jobs are defined via NATS key-value storage and executed reliably by a single active instance, with automatic failover and dynamic updates.
