# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build tools
RUN apk add --no-cache git

# Install sqlc
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Copy go.mod and go.sum first to leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the rest of the application source code
COPY . .

# Generate SQLC code
RUN sqlc generate

# Build the application
# CGO_ENABLED=0 produces a statically linked binary (important for Alpine)
# -ldflags "-s -w" strips debug information and symbols, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags "-s -w" -o /app/lockin-bot ./main.go

# Stage 2: Create the final lightweight image
FROM alpine:3.21

# Add ca-certificates for HTTPS requests (Discord API)
RUN apk add --no-cache ca-certificates

# Create non-root user for security
RUN addgroup -g 1000 -S lockin && \
    adduser -u 1000 -S lockin -G lockin

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/lockin-bot /app/lockin-bot

# Copy the database migrations
COPY --from=builder /app/db/migrations ./db/migrations

# Change ownership to non-root user
RUN chown -R lockin:lockin /app

# Switch to non-root user
USER lockin

# Set the command to run the application
# The application will pick up environment variables set in Render.
CMD ["/app/lockin-bot"]
