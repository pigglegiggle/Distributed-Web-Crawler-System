FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -o crawler .

FROM alpine:3.20
WORKDIR /app
RUN adduser -D appuser
COPY --from=builder /app/crawler /app/crawler
RUN mkdir -p /app/data && chown -R appuser:appuser /app
USER appuser

ENTRYPOINT ["/app/crawler"]
