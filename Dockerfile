FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the ralph binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /ralph cmd/ralph/main.go

# --- Main image ---
FROM alpine:3.21

# Install shells and git for testing
RUN apk add --no-cache bash zsh fish git

# Copy the ralph binary from the builder stage
COPY --from=builder /ralph /usr/local/bin/ralph

# Set up a non-root user for tests to run as (good practice)
RUN addgroup -S testgroup && adduser -S testuser -G testgroup
USER testuser
WORKDIR /home/testuser

# Default entrypoint (can be overridden by docker run commands)
ENTRYPOINT ["/usr/local/bin/ralph"] 