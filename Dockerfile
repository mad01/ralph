FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the dotter binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /dotter cmd/dotter/main.go

# --- Main image ---
FROM alpine:latest

# Install shells and git for testing
RUN apk add --no-cache bash zsh fish git

# Copy the dotter binary from the builder stage
COPY --from=builder /dotter /usr/local/bin/dotter

# Set up a non-root user for tests to run as (good practice)
RUN addgroup -S testgroup && adduser -S testuser -G testgroup
USER testuser
WORKDIR /home/testuser

# Default entrypoint (can be overridden by docker run commands)
ENTRYPOINT ["/usr/local/bin/dotter"] 