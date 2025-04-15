FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata && update-ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o fulfiller .

# Create final lightweight image
FROM alpine:latest

WORKDIR /app

# Copy the binary and certificates from builder
COPY --from=builder /app/fulfiller .
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Set environment variables
ENV TZ=UTC

# Run as non-privileged user
RUN adduser -D -g '' appuser
USER appuser

# Command to run the application
ENTRYPOINT ["/app/fulfiller"] 