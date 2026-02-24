# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o notification ./cmd/notification

# Run stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates
RUN mkdir -p /data

WORKDIR /app

COPY --from=builder /app/notification .

VOLUME /data

EXPOSE 8086

CMD ["./notification"]
