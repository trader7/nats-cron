FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY simple-worker.go .
COPY go.mod go.sum ./

RUN go mod tidy
RUN go build -o worker simple-worker.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/worker .
CMD ["./worker"]