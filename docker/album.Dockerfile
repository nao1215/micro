# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o album ./cmd/album

# Run stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates
RUN mkdir -p /data

WORKDIR /app

COPY --from=builder /app/album .

VOLUME /data

EXPOSE 8083

CMD ["./album"]
