# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod file
COPY go.mod ./

# Copy source code
COPY . .

# Download dependencies (creates go.sum if needed)
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o portus ./cmd/portus

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/portus .

# Create config directory
RUN mkdir -p /app/config/models

# Expose port
EXPOSE 8080

# Run the application
CMD ["./portus"]
