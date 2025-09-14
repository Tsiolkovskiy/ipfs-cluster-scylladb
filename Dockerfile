# Multi-stage build for IPFS-Cluster with ScyllaDB state storage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the IPFS-Cluster daemon with ScyllaDB support
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o ipfs-cluster-service ./cmd/ipfs-cluster-service
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o ipfs-cluster-ctl ./cmd/ipfs-cluster-ctl

# Final stage - minimal runtime image
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache ca-certificates su-exec tini

# Create cluster user
RUN adduser -D -s /bin/sh cluster

# Create directories
RUN mkdir -p /data/ipfs-cluster && \
    chown cluster:cluster /data/ipfs-cluster

# Copy binaries from builder
COPY --from=builder /app/ipfs-cluster-service /usr/local/bin/
COPY --from=builder /app/ipfs-cluster-ctl /usr/local/bin/

# Copy configuration templates
COPY --from=builder /app/examples/configurations /etc/ipfs-cluster/configurations

# Set permissions
RUN chmod +x /usr/local/bin/ipfs-cluster-service /usr/local/bin/ipfs-cluster-ctl

# Expose ports
EXPOSE 9094 9095 9096 8888

# Set default environment variables
ENV CLUSTER_PATH=/data/ipfs-cluster
ENV CLUSTER_LOGLEVEL=info

# Create entrypoint script
RUN echo '#!/bin/sh' > /entrypoint.sh && \
    echo 'set -e' >> /entrypoint.sh && \
    echo '' >> /entrypoint.sh && \
    echo '# Initialize cluster if no configuration exists' >> /entrypoint.sh && \
    echo 'if [ ! -f "$CLUSTER_PATH/service.json" ]; then' >> /entrypoint.sh && \
    echo '    echo "Initializing IPFS-Cluster..."' >> /entrypoint.sh && \
    echo '    su-exec cluster ipfs-cluster-service init' >> /entrypoint.sh && \
    echo 'fi' >> /entrypoint.sh && \
    echo '' >> /entrypoint.sh && \
    echo '# Start the cluster service' >> /entrypoint.sh && \
    echo 'exec su-exec cluster ipfs-cluster-service daemon "$@"' >> /entrypoint.sh && \
    chmod +x /entrypoint.sh

# Use tini as init system
ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD ipfs-cluster-ctl --host /ip4/127.0.0.1/tcp/9094 id || exit 1

# Set user
USER cluster

# Set working directory
WORKDIR /data/ipfs-cluster