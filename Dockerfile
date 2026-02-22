# Multi-stage Dockerfile for whm2bunny
# Stage 1: Build
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X github.com/mordenhost/whm2bunny/cmd/whm2bunny/commands.Version=${VERSION} -X github.com/mordenhost/whm2bunny/cmd/whm2bunny/commands.Commit=${COMMIT} -X github.com/mordenhost/whm2bunny/cmd/whm2bunny/commands.BuildTime=${BUILD_TIME}" \
    -o whm2bunny ./cmd/whm2bunny

# Verify the binary
RUN ./whm2bunny version

# Stage 2: Runtime
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    curl \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 whm2bunny && \
    adduser -D -u 1000 -G whm2bunny whm2bunny

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder --chown=whm2bunny:whm2bunny /build/whm2bunny /usr/local/bin/whm2bunny

# Copy config example
COPY --from=builder --chown=whm2bunny:whm2bunny /build/config.yaml.example /etc/whm2bunny/config.yaml.example

# Create directories
RUN mkdir -p /var/lib/whm2bunny /var/log/whm2bunny /etc/whm2bunny && \
    chown -R whm2bunny:whm2bunny /var/lib/whm2bunny /var/log/whm2bunny /etc/whm2bunny

# Switch to non-root user
USER whm2bunny

# Expose port
EXPOSE 9090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:9090/health || exit 1

# Default command
ENTRYPOINT ["whm2bunny"]
CMD ["serve", "--config", "/etc/whm2bunny/config.yaml"]
