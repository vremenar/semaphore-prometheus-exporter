# --- Build stage ---
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy go.mod and fetch dependencies (go.sum generated here, not committed to repo)
COPY go.mod ./
RUN go mod tidy

# Copy source and build
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o semaphore-prometheus-exporter .

# --- Runtime stage ---
FROM scratch

LABEL org.opencontainers.image.title="semaphore-prometheus-exporter" \
      org.opencontainers.image.description="Prometheus exporter for Semaphore UI" \
      org.opencontainers.image.source="https://github.com/vremenar/semaphore-prometheus-exporter"

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /build/semaphore-prometheus-exporter /semaphore-prometheus-exporter

VOLUME ["/opt/semaphore-prometheus-exporter/data"]

EXPOSE 9090

ENTRYPOINT ["/semaphore-prometheus-exporter"]
