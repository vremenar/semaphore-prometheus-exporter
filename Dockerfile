# --- Build stage ---
FROM golang:1.26-alpine AS builder

# Version is injected by Docker Buildx from the workflow (git tag or manual value)
ARG APP_VERSION=dev

WORKDIR /build

# Install wget for healthcheck
RUN apk update && apk add --no-cache wget

# Copy everything at once so go mod tidy can see all imports
COPY go.mod *.go ./
COPY static/ ./static/

# Fetch dependencies based on actual imports, then verify
RUN go mod tidy && go mod download && go mod verify

# Build — override the Version constant via ldflags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -v -trimpath \
    -ldflags="-s -w -X main.Version=${APP_VERSION}" \
    -o semaphore-prometheus-exporter .

# --- Runtime stage ---
FROM scratch

LABEL org.opencontainers.image.title="semaphore-prometheus-exporter" \
      org.opencontainers.image.description="Prometheus exporter for Semaphore UI" \
      org.opencontainers.image.source="https://github.com/vremenar/semaphore-prometheus-exporter"

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/bin/wget /usr/bin/wget
COPY --from=builder /build/semaphore-prometheus-exporter /semaphore-prometheus-exporter

VOLUME ["/opt/semaphore-prometheus-exporter/data"]

EXPOSE 9090

ENTRYPOINT ["/semaphore-prometheus-exporter"]