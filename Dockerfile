# ─── Stage 1: Build the Go binary ─────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency files first for better Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# ─── Stage 2: Minimal runtime image ──────────────────────────────
FROM alpine:3.19

# Install Docker CLI so we can run `docker exec` against the runner container
RUN apk add --no-cache docker-cli ca-certificates

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/server .
# Copy templates for runtime access
COPY --from=builder /app/templates ./templates

# Create workspace directory
RUN mkdir -p /app/workspaces

EXPOSE 8080

CMD ["./server"]
