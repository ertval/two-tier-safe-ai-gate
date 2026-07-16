FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy dependency configs
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build standard binary
RUN CGO_ENABLED=0 GOOS=linux go build -o two-tier-safe-ai-gate .

FROM alpine:latest

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/two-tier-safe-ai-gate .

# Expose Go server port
EXPOSE 8090

# Default command runs the Inngest handler and REST server
CMD ["./two-tier-safe-ai-gate"]
