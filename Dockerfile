FROM golang:1.18.1-alpine AS builder

WORKDIR /src
COPY . .

RUN apk add --no-cache git && \
    go mod download && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "gosses"

FROM alpine:3.16.3

WORKDIR /

COPY --from=builder "/src/gosses" "/"

ENTRYPOINT ["/gosses"]
EXPOSE 8001
