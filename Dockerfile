# Stage 1: Build the binary
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bot ./cmd/bot

# Stage 2: Minimal runtime image
FROM alpine:latest

WORKDIR /app

# Install CA certificates for secure HTTPS API calls
RUN apk --no-cache add ca-certificates

# Copy the binary and env configurations from the build stage
COPY --from=builder /app/bot .
COPY --from=builder /app/.env .

# Expose default port
EXPOSE 8080

# Run binary
CMD ["./bot"]
