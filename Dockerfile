FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o server ./cmd/server

# Runtime stage
FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/server ./server

ENV APP_PORT=3000
EXPOSE 3000

ENTRYPOINT ["/app/server"]
