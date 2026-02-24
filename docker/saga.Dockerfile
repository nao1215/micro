# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o saga ./cmd/saga

# Run stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates sqlite-libs
RUN mkdir -p /data

WORKDIR /app

COPY --from=builder /app/saga .

VOLUME /data

EXPOSE 8085

CMD ["./saga"]
