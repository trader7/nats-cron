# Job Templates

Common job patterns for NATS Cron. Copy and modify these templates for your use cases.

## Database Maintenance

### Daily Cleanup
```json
{
  "id": "cleanup-{table}",
  "target": {
    "subject": "database.cleanup"
  },
  "payload": {
    "type": "json",
    "data": "{\"table\":\"{table_name}\",\"older_than\":\"{retention_period}\",\"batch_size\":1000}"
  },
  "schedule": {
    "cron": "0 2 * * *"
  }
}
```

### Database Backup
```json
{
  "id": "backup-{database}",
  "target": {
    "subject": "database.backup"
  },
  "payload": {
    "type": "json",
    "data": "{\"database\":\"{db_name}\",\"destination\":\"{backup_location}\"}"
  },
  "schedule": {
    "cron": "0 1 * * *"
  }
}
```

## Monitoring & Health Checks

### Service Health Check
```json
{
  "id": "health-{service}",
  "target": {
    "subject": "monitoring.health_check"
  },
  "payload": {
    "type": "json",
    "data": "{\"service\":\"{service_name}\",\"endpoint\":\"{health_endpoint}\",\"timeout\":\"10s\"}"
  },
  "schedule": {
    "every": "30s"
  }
}
```

### Log Analysis
```json
{
  "id": "analyze-logs",
  "target": {
    "subject": "monitoring.log_analysis"
  },
  "payload": {
    "type": "json",
    "data": "{\"log_path\":\"{path}\",\"pattern\":\"{error_pattern}\",\"alert_threshold\":10}"
  },
  "schedule": {
    "every": "5m"
  }
}
```

## Reports & Analytics

### Daily Report
```json
{
  "id": "daily-{report_type}",
  "target": {
    "subject": "reports.generate"
  },
  "payload": {
    "type": "json",
    "data": "{\"type\":\"{report_type}\",\"period\":\"daily\",\"recipients\":[\"{email}\"]}"
  },
  "schedule": {
    "cron": "0 8 * * *"
  }
}
```

### Weekly Summary
```json
{
  "id": "weekly-summary",
  "target": {
    "subject": "reports.generate"
  },
  "payload": {
    "type": "json",
    "data": "{\"type\":\"summary\",\"period\":\"weekly\",\"format\":\"pdf\",\"recipients\":[\"{email}\"]}"
  },
  "schedule": {
    "cron": "0 9 * * MON"
  }
}
```

## Cache & Performance

### Cache Warming
```json
{
  "id": "warm-{cache_name}",
  "target": {
    "subject": "cache.warm"
  },
  "payload": {
    "type": "json",
    "data": "{\"cache_keys\":[\"{key1}\",\"{key2}\"],\"ttl\":\"1h\"}"
  },
  "schedule": {
    "every": "15m"
  }
}
```

### Cache Invalidation
```json
{
  "id": "invalidate-cache",
  "target": {
    "subject": "cache.invalidate"
  },
  "payload": {
    "type": "json",
    "data": "{\"pattern\":\"{cache_pattern}\",\"reason\":\"scheduled_refresh\"}"
  },
  "schedule": {
    "cron": "0 0 * * *"
  }
}
```

## Data Processing

### ETL Pipeline
```json
{
  "id": "etl-{pipeline_name}",
  "target": {
    "subject": "pipeline.trigger"
  },
  "payload": {
    "type": "json",
    "data": "{\"pipeline\":\"{name}\",\"source\":\"{source_table}\",\"destination\":\"{dest_table}\"}"
  },
  "schedule": {
    "cron": "0 * * * *"
  }
}
```

### Data Sync
```json
{
  "id": "sync-{service}",
  "target": {
    "subject": "data.sync"
  },
  "payload": {
    "type": "json",
    "data": "{\"source\":\"{source_api}\",\"destination\":\"{dest_db}\",\"batch_size\":1000}"
  },
  "schedule": {
    "every": "10m"
  }
}
```

## Security & Compliance

### Security Scan
```json
{
  "id": "security-scan",
  "target": {
    "subject": "security.scan"
  },
  "payload": {
    "type": "json",
    "data": "{\"target\":\"{scan_target}\",\"scan_type\":\"vulnerability\",\"notify\":true}"
  },
  "schedule": {
    "cron": "0 3 * * *"
  }
}
```

### Audit Log Rotation
```json
{
  "id": "rotate-audit-logs",
  "target": {
    "subject": "logs.rotate"
  },
  "payload": {
    "type": "json",
    "data": "{\"log_type\":\"audit\",\"retention_days\":90,\"compress\":true}"
  },
  "schedule": {
    "cron": "0 0 1 * *"
  }
}
```

## Usage Tips

1. Replace `{placeholders}` with your actual values
2. Use descriptive job IDs that include the resource being operated on
3. Choose appropriate schedules - not everything needs to run every minute
4. Include enough context in the payload for workers to operate independently
5. Use consistent subject naming conventions across your organization