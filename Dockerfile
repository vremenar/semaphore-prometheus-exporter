# --- Build stage ---
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Cache Go modules (go.sum is generated automatically if not present)
COPY go.mod ./
COPY go.sum* ./
RUN go mod tidy

# Build the binary
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o semaphore-exporter .

# --- Runtime stage ---
FROM scratch

LABEL org.opencontainers.image.title="semaphore-exporter" \
      org.opencontainers.image.description="Prometheus exporter for Semaphore UI" \
      org.opencontainers.image.source="https://github.com/example/semaphore-exporter"

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /build/semaphore-exporter /semaphore-exporter

# Default cache directory
VOLUME ["/opt/semaphore-exporter/data"]

EXPOSE 9090

ENTRYPOINT ["/semaphore-exporter"]
