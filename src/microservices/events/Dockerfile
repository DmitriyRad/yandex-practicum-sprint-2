FROM golang:1.23 AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o events-service .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/events-service .
EXPOSE 8082
CMD ["./events-service"]