# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mysql-mcp-server .

# Final stage - minimal image
FROM scratch

# Copy the binary
COPY --from=builder /app/mysql-mcp-server /mysql-mcp-server

# Copy CA certificates for TLS connections to MySQL
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/mysql-mcp-server"]
