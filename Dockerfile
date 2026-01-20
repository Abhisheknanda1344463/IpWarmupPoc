# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS calls and Chromium for CAPTCHA detection
RUN apk --no-cache add ca-certificates chromium

# Set Chromium path for chromedp
ENV CHROME_PATH=/usr/bin/chromium-browser

# Copy binary from builder
COPY --from=builder /app/main .
COPY --from=builder /app/index.html .

# Expose port
EXPOSE 8080

# Run the binary
CMD ["./main"]

