FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o hub-backend .

# Use a minimal alpine image
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/hub-backend .

# Expose the API port
EXPOSE 4000

# Run the backend registry
ENTRYPOINT ["./hub-backend"]
CMD ["start"]
