# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o eventstore ./cmd/eventstore

# Run stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates
RUN mkdir -p /data

WORKDIR /app

COPY --from=builder /app/eventstore .

VOLUME /data

EXPOSE 8084

CMD ["./eventstore"]
