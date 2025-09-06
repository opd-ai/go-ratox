# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /src

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN make build

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN adduser -D -s /bin/sh ratox

# Set working directory
WORKDIR /home/ratox

# Copy binary from builder stage
COPY --from=builder /src/build/ratox-go /usr/local/bin/ratox-go

# Create config directory
RUN mkdir -p /home/ratox/.config/ratox-go && \
    chown -R ratox:ratox /home/ratox

# Switch to non-root user
USER ratox

# Set default config directory
ENV RATOX_CONFIG_DIR=/home/ratox/.config/ratox-go

# Expose config directory as volume
VOLUME ["/home/ratox/.config/ratox-go"]

# Default command
CMD ["ratox-go", "-p", "/home/ratox/.config/ratox-go"]
