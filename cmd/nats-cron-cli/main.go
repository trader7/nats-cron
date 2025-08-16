package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/trader7/nats-cron/pkg/config"
)

type JobStatus struct {
	Subject string    `json:"subject"`
	LastRun time.Time `json:"last_run"`
	NextRun time.Time `json:"next_run"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg := config.New()
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to NATS: %v\n", err)
		os.Exit(1)
	}
	defer nc.Close()

	switch os.Args[1] {
	case "list", "ls":
		listJobs(nc)
	case "status":
		showStatus(nc)
	case "add":
		addSimpleJob(nc, os.Args[2:])
	case "create":
		createJob(nc, os.Args[2:])
	case "update":
		updateJob(nc, os.Args[2:])
	case "delete", "del":
		deleteJob(nc, os.Args[2:])
	case "get":
		getJob(nc, os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("NATS Cron CLI - Manage scheduled jobs")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  nats-cron-cli <command> [args...]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  list, ls                    List all jobs")
	fmt.Println("  status                      Show service status")
	fmt.Println("  add <subject> <interval> [data]  Add a simple job")
	fmt.Println("  create <json-file>          Create a job from JSON file")
	fmt.Println("  update <json-file>          Update a job from JSON file")
	fmt.Println("  delete <subject>            Delete a job")
	fmt.Println("  get <subject>               Get job details")
	fmt.Println("  help                        Show this help")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  nats-cron-cli add system.ping 30s")
	fmt.Println("  nats-cron-cli add db.cleanup 1h '{\"table\":\"sessions\"}'")
	fmt.Println("  nats-cron-cli add reports.daily \"0 9 * * *\"")
	fmt.Println("  nats-cron-cli delete \"test.*\"        # Delete all test jobs")
	fmt.Println("  nats-cron-cli delete \"orders.>\"      # Delete all orders jobs")
	fmt.Println()
	fmt.Println("Intervals:")
	fmt.Println("  - Time duration: 5s, 10m, 1h, etc.")
	fmt.Println("  - Cron expression: \"0 9 * * *\" (daily at 9am)")
	fmt.Println()
	fmt.Println("NATS Wildcards (for delete):")
	fmt.Println("  - * matches exactly one token: orders.*.created")
	fmt.Println("  - > matches one or more tokens: orders.>")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  NATS_URL                    NATS server URL (default: nats://localhost:4222)")
}

func listJobs(nc *nats.Conn) {
	msg, err := nc.Request("nats-cron.jobs.list", nil, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing jobs: %v\n", err)
		os.Exit(1)
	}

	var response struct {
		Jobs []JobStatus `json:"jobs"`
	}

	if err := json.Unmarshal(msg.Data, &response); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(response.Jobs) == 0 {
		fmt.Println("No jobs found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SUBJECT\tLAST RUN\tNEXT RUN\n")
	for _, job := range response.Jobs {
		lastRun := "never"
		if !job.LastRun.IsZero() {
			lastRun = job.LastRun.Format(time.RFC3339)
		}
		nextRun := "unknown"
		if !job.NextRun.IsZero() {
			nextRun = job.NextRun.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", job.Subject, lastRun, nextRun)
	}
	w.Flush()
}

func showStatus(nc *nats.Conn) {
	msg, err := nc.Request("nats-cron.status", nil, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting status: %v\n", err)
		os.Exit(1)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(msg.Data, &status); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing status: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Service: %v\n", status["service"])
	fmt.Printf("Is Leader: %v\n", status["is_leader"])
	fmt.Printf("Active Jobs: %v\n", status["jobs"])
}

func createJob(nc *nats.Conn, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing job file\n")
		os.Exit(1)
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading job file: %v\n", err)
		os.Exit(1)
	}

	msg, err := nc.Request("nats-cron.jobs.create", data, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating job: %v\n", err)
		os.Exit(1)
	}

	var response map[string]string
	json.Unmarshal(msg.Data, &response)
	fmt.Printf("Job created: %s\n", response["status"])
}

func updateJob(nc *nats.Conn, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing job file\n")
		os.Exit(1)
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading job file: %v\n", err)
		os.Exit(1)
	}

	msg, err := nc.Request("nats-cron.jobs.update", data, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating job: %v\n", err)
		os.Exit(1)
	}

	var response map[string]string
	json.Unmarshal(msg.Data, &response)
	fmt.Printf("Job updated: %s\n", response["status"])
}

func deleteJob(nc *nats.Conn, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing job subject\n")
		os.Exit(1)
	}

	subject := args[0]

	// Check if this is a wildcard pattern
	if strings.Contains(subject, "*") || strings.Contains(subject, ">") {
		deleteJobsWithPattern(nc, subject)
		return
	}

	// Single job deletion
	request := map[string]string{"subject": subject}
	data, _ := json.Marshal(request)

	msg, err := nc.Request("nats-cron.jobs.delete", data, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting job: %v\n", err)
		os.Exit(1)
	}

	var response map[string]string
	json.Unmarshal(msg.Data, &response)

	if errMsg, exists := response["error"]; exists {
		fmt.Fprintf(os.Stderr, "Error: %s\n", errMsg)
		os.Exit(1)
	}

	fmt.Printf("Job deleted: %s\n", subject)
}

func deleteJobsWithPattern(nc *nats.Conn, pattern string) {
	// First, get list of jobs to see what matches
	msg, err := nc.Request("nats-cron.jobs.list", nil, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing jobs: %v\n", err)
		os.Exit(1)
	}

	var response struct {
		Jobs []JobStatus `json:"jobs"`
	}
	if err := json.Unmarshal(msg.Data, &response); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	// Find matching jobs
	var matches []string
	for _, job := range response.Jobs {
		if matchesPattern(job.Subject, pattern) {
			matches = append(matches, job.Subject)
		}
	}

	if len(matches) == 0 {
		fmt.Printf("No jobs match pattern '%s'\n", pattern)
		return
	}

	// Show matches and confirm
	fmt.Printf("Found %d job(s) matching pattern '%s':\n", len(matches), pattern)
	for _, subject := range matches {
		fmt.Printf("  - %s\n", subject)
	}
	fmt.Printf("\nDelete these %d job(s)? (y/N): ", len(matches))

	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" && confirm != "yes" {
		fmt.Println("Deletion cancelled")
		return
	}

	// Delete each job
	deleted := 0
	failed := 0
	for _, subject := range matches {
		request := map[string]string{"subject": subject}
		data, _ := json.Marshal(request)

		msg, err := nc.Request("nats-cron.jobs.delete", data, 5*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting job %s: %v\n", subject, err)
			failed++
			continue
		}

		var response map[string]string
		json.Unmarshal(msg.Data, &response)

		if errMsg, exists := response["error"]; exists {
			fmt.Fprintf(os.Stderr, "Error deleting job %s: %s\n", subject, errMsg)
			failed++
		} else {
			deleted++
		}
	}

	fmt.Printf("\nDeleted %d job(s)", deleted)
	if failed > 0 {
		fmt.Printf(" (%d failed)", failed)
	}
	fmt.Println()
}

// matchesPattern implements NATS wildcard pattern matching for CLI
func matchesPattern(subject, pattern string) bool {
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

	si, pi := 0, 0

	for pi < len(patternTokens) && si < len(subjectTokens) {
		switch patternTokens[pi] {
		case "*":
			// * matches exactly one token
			si++
			pi++
		case ">":
			// > matches remaining tokens, must be last in pattern
			return pi == len(patternTokens)-1
		default:
			// Literal token must match exactly
			if subjectTokens[si] != patternTokens[pi] {
				return false
			}
			si++
			pi++
		}
	}

	// Check if we consumed all tokens correctly
	if pi < len(patternTokens) {
		// Remaining pattern tokens
		if len(patternTokens)-pi == 1 && patternTokens[pi] == ">" {
			// Pattern ends with >, matches any remaining subject tokens
			return true
		}
		// Unmatched pattern tokens (not ending with >)
		return false
	}

	// All pattern tokens consumed, subject should also be fully consumed
	return si == len(subjectTokens)
}

// isValidCronExpression validates a cron expression format
func isValidCronExpression(expr string) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}

	// Basic validation of each field
	for i, field := range fields {
		if !isValidCronField(field, i) {
			return false
		}
	}

	return true
}

// isValidCronField validates individual cron fields
func isValidCronField(field string, fieldType int) bool {
	// Allow basic patterns: *, numbers, ranges (1-5), lists (1,3,5), steps (*/2, 1-10/2)
	if field == "*" {
		return true
	}

	// Check for step notation (*/n or range/n)
	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return false
		}
		// Second part must be a number
		if !isPositiveInteger(parts[1]) {
			return false
		}
		// First part can be * or a range
		if parts[0] != "*" && !isValidCronField(parts[0], fieldType) {
			return false
		}
		return true
	}

	// Check for ranges (1-5)
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return false
		}
		return isPositiveInteger(parts[0]) && isPositiveInteger(parts[1])
	}

	// Check for lists (1,3,5)
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, part := range parts {
			if !isValidCronField(part, fieldType) {
				return false
			}
		}
		return true
	}

	// Must be a number
	return isPositiveInteger(field)
}

// isPositiveInteger checks if string is a positive integer
func isPositiveInteger(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isValidDuration validates Go duration format
func isValidDuration(d string) bool {
	if d == "" {
		return false
	}

	// Try parsing with Go's time.ParseDuration
	_, err := time.ParseDuration(d)
	return err == nil
}

func getJob(nc *nats.Conn, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing job subject\n")
		os.Exit(1)
	}

	request := map[string]string{"subject": args[0]}
	data, _ := json.Marshal(request)

	msg, err := nc.Request("nats-cron.jobs.get", data, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting job: %v\n", err)
		os.Exit(1)
	}

	var job map[string]interface{}
	if err := json.Unmarshal(msg.Data, &job); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing job: %v\n", err)
		os.Exit(1)
	}

	prettyJob, _ := json.MarshalIndent(job, "", "  ")
	fmt.Println(string(prettyJob))
}

func addSimpleJob(nc *nats.Conn, args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Error: add requires at least 2 arguments\n")
		fmt.Fprintf(os.Stderr, "Usage: nats-cron-cli add <subject> <interval> [data]\n")
		os.Exit(1)
	}

	subject := args[0]
	interval := args[1]
	data := ""

	if len(args) > 2 {
		data = args[2]
	}

	// Build job definition
	job := map[string]interface{}{
		"target": map[string]interface{}{
			"subject": subject,
		},
		"payload": map[string]interface{}{
			"type": "json",
			"data": data,
		},
	}

	// Validate and determine if interval is cron or duration
	schedule := map[string]interface{}{}
	if isCronExpression(interval) {
		// Validate cron expression
		if !isValidCronExpression(interval) {
			fmt.Fprintf(os.Stderr, "Error: invalid cron expression '%s'\n", interval)
			fmt.Fprintf(os.Stderr, "Cron format: 'minute hour day month weekday' (e.g., '0 9 * * *')\n")
			os.Exit(1)
		}
		schedule["cron"] = interval
	} else {
		// Validate duration
		if !isValidDuration(interval) {
			fmt.Fprintf(os.Stderr, "Error: invalid duration '%s'\n", interval)
			fmt.Fprintf(os.Stderr, "Duration format: number + unit (e.g., '30s', '5m', '2h')\n")
			os.Exit(1)
		}
		schedule["every"] = interval
	}
	job["schedule"] = schedule

	// Create the job
	jobData, _ := json.Marshal(job)

	msg, err := nc.Request("nats-cron.jobs.create", jobData, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating job: %v\n", err)
		os.Exit(1)
	}

	var response map[string]string
	json.Unmarshal(msg.Data, &response)

	if errMsg, exists := response["error"]; exists {
		fmt.Fprintf(os.Stderr, "Error: %s\n", errMsg)
		os.Exit(1)
	}

	fmt.Printf("Job '%s' created successfully\n", subject)
	fmt.Printf("  Schedule: %s\n", interval)
	if data != "" {
		fmt.Printf("  Data: %s\n", data)
	}
}

func isCronExpression(s string) bool {
	// Check if it contains spaces (cron expressions have spaces between fields)
	// and doesn't look like a duration (no time units like s, m, h)
	if !strings.Contains(s, " ") {
		return false
	}

	// Check that it doesn't end with duration suffixes
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "m") ||
		strings.HasSuffix(s, "h") || strings.HasSuffix(s, "ms") ||
		strings.HasSuffix(s, "us") || strings.HasSuffix(s, "ns") {
		return false
	}

	// If it has spaces and no duration suffix, treat it as a potential cron expression
	// The actual validation will happen in isValidCronExpression
	return true
}
