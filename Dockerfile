# Stage 1: Build the Go application
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

# Copy the rest of the application source code
COPY . .

# Build the application
# CGO_ENABLED=0 produces a statically linked binary (important for Alpine)
# -ldflags "-s -w" strips debug information and symbols, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags "-s -w" -o /app/lockin-bot ./main.go

# Stage 2: Create the final lightweight image
FROM alpine:latest

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/lockin-bot /app/lockin-bot

# Copy the database migrations
# Ensure this path matches your project structure relative to the Dockerfile
COPY db/migrations ./db/migrations

# (Optional) If you had other assets like config files not handled by env vars, copy them here too.
# COPY config.production.json ./config.json

# Set the command to run the application
# The application will pick up environment variables set in Render.
CMD ["/app/lockin-bot"] 