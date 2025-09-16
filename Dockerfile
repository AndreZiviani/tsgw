# Multi-stage Dockerfile for TSGW (Tailscale Gateway)
# Stage 1: Build the Go application
FROM golang:1.25.1-alpine AS builder

# Install git and ca-certificates (needed for Go modules and HTTPS requests)
RUN apk add --no-cache git ca-certificates

# Set the working directory inside the container
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application with optimizations
# CGO_ENABLED=0 for static linking, -ldflags for smaller binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tsgw .

# Stage 2: Create the runtime image
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create a non-root user for security
RUN addgroup -S tsgw && adduser -S tsgw -G tsgw

# Set the working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/tsgw .

# Change ownership of the application files to the non-root user
RUN chown -R tsgw:tsgw /app

# Switch to the non-root user
USER tsgw

# Expose the default HTTPS port
EXPOSE 443

# Set the default command
CMD ["./tsgw"]
