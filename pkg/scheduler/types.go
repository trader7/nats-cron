package scheduler

import "time"

type JobDefinition struct {
	Target struct {
		Subject string `json:"subject"`
	} `json:"target"`
	Payload struct {
		Type string `json:"type"`
		Data string `json:"data"`
	} `json:"payload"`
	Schedule struct {
		Every string `json:"every,omitempty"`
		Cron  string `json:"cron,omitempty"`
	} `json:"schedule"`
	LastRun time.Time `json:"last_run,omitempty"`
	NextRun time.Time `json:"next_run,omitempty"`
}

type JobStatus struct {
	Subject string    `json:"subject"`
	LastRun time.Time `json:"last_run"`
	NextRun time.Time `json:"next_run"`
}