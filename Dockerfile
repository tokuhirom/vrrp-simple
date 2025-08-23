FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vrrp ./main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    iproute2 \
    iptables

# Copy binary from builder
COPY --from=builder /app/vrrp /usr/local/bin/vrrp

# Make binary executable
RUN chmod +x /usr/local/bin/vrrp

# Create non-root user (optional, VRRP needs NET_ADMIN capability)
# RUN addgroup -g 1000 vrrp && \
#     adduser -D -u 1000 -G vrrp vrrp

# Default command
ENTRYPOINT ["vrrp"]
CMD ["--help"]