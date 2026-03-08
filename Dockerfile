FROM golang:alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o babelsuite .

# Use a minimal alpine image for the final stage
FROM alpine:latest

# Install docker-cli so the engine can interact with the host docker daemon if needed
RUN apk add --no-cache docker-cli

WORKDIR /app

COPY --from=builder /app/babelsuite .

# Expose the API port
EXPOSE 3000

# Run the daemon by default
ENTRYPOINT ["./babelsuite"]
CMD ["daemon"]
