# Stage 1: Build statically linked, stripped Go binaries
FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build stripped static binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/orchestrator ./cmd/orchestrator
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/orchctl ./cmd/orchctl

# Stage 2a: Orchestrator node image
FROM alpine:3.20 AS orchestrator
RUN apk add --no-cache curl ca-certificates
WORKDIR /app
COPY --from=builder /bin/orchestrator /app/orchestrator
COPY --from=builder /bin/orchctl /app/orchctl

EXPOSE 50051 8080
ENTRYPOINT ["/app/orchestrator"]

# Stage 2b: Worker daemon image
FROM alpine:3.20 AS worker
RUN apk add --no-cache bash curl ca-certificates
WORKDIR /app
COPY --from=builder /bin/worker /app/worker

EXPOSE 8081
ENTRYPOINT ["/app/worker"]
