FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o minecraft-manager cmd/client/main.go

# Final stage - using Ubuntu for better Bedrock server compatibility
FROM ubuntu:22.04

# Install necessary packages
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    wget \
    unzip \
    && rm -rf /var/lib/apt/lists/*

# Create app user
RUN groupadd -r appgroup && useradd -r -g appgroup appuser

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/minecraft-manager .

# Copy configuration files
COPY --from=builder /app/config.yaml .
COPY --from=builder /app/example-servers.yaml .

# Create servers directory
RUN mkdir -p servers && chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose HTTP port
EXPOSE 8080

# Expose common Bedrock ports
EXPOSE 19132-19136

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./minecraft-manager"] 