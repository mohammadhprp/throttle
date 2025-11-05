FROM golang:1.24-alpine

WORKDIR /build

ENV GOSUMDB=off

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags="-s -w" -o server .

ENTRYPOINT ["/build/server"]