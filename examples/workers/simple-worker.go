package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/nats-io/nats.go"
)

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}
	defer nc.Close()

	// Subscribe to different job types
	subscribeToJobs(nc, "database.cleanup", handleCleanupJob)
	subscribeToJobs(nc, "reports.generate", handleReportJob)
	subscribeToJobs(nc, "cache.warm", handleCacheJob)
	subscribeToJobs(nc, "monitoring.health_check", handleHealthCheck)

	log.Println("Worker started. Waiting for jobs...")
	select {} // Keep running
}

func subscribeToJobs(nc *nats.Conn, subject string, handler func(map[string]interface{})) {
	nc.Subscribe(subject, func(msg *nats.Msg) {
		var jobData map[string]interface{}
		if err := json.Unmarshal(msg.Data, &jobData); err != nil {
			log.Printf("Invalid job payload on %s: %v", subject, err)
			return
		}
		
		log.Printf("Received job on %s: %v", subject, jobData)
		handler(jobData)
	})
}

func handleCleanupJob(jobData map[string]interface{}) {
	table := jobData["table"].(string)
	olderThan := jobData["older_than"].(string)
	
	log.Printf("Cleaning up table: %s, older than: %s", table, olderThan)
	
	// Do cleanup work here...
	// Example: Connect to database, run DELETE query
	
	log.Printf("Cleanup completed for table: %s", table)
}

func handleReportJob(jobData map[string]interface{}) {
	reportType := jobData["type"].(string)
	period := jobData["period"].(string)
	
	log.Printf("Generating %s report for period: %s", reportType, period)
	
	// Do report generation here...
	// Example: Query database, create PDF/CSV, send email
	
	log.Printf("Report generated: %s %s", reportType, period)
}

func handleCacheJob(jobData map[string]interface{}) {
	cacheKeys := jobData["cache_keys"].([]interface{})
	
	log.Printf("Warming cache for keys: %v", cacheKeys)
	
	// Do cache warming here...
	// Example: Fetch data from database, store in Redis
	
	log.Printf("Cache warmed for %d keys", len(cacheKeys))
}

func handleHealthCheck(jobData map[string]interface{}) {
	service := jobData["service"].(string)
	endpoint := jobData["endpoint"].(string)
	
	log.Printf("Health checking %s at %s", service, endpoint)
	
	// Do health check here...
	// Example: HTTP GET request, check response code
	
	log.Printf("Health check completed for: %s", service)
}