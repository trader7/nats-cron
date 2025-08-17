# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o nats-cron-server ./cmd/nats-cron-server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o nats-cron ./cmd/nats-cron

# Final stage
FROM alpine:latest

# Install ca-certificates for TLS support
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binaries from builder stage
COPY --from=builder /app/nats-cron-server .
COPY --from=builder /app/nats-cron .

# Create non-root user
RUN adduser -D -s /bin/sh nats-cron
USER nats-cron

# Expose default NATS port (though this service doesn't listen)
EXPOSE 4222

# Default command
CMD ["./nats-cron-server"]