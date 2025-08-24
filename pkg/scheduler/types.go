package scheduler

type JobDefinition struct {
	Subject  string `json:"subject"`
	Schedule struct {
		Every string `json:"every,omitempty"`
		Cron  string `json:"cron,omitempty"`
	} `json:"schedule"`
}

type JobStatus struct {
	Subject string `json:"subject"`
}
