FROM golang:1.18.0-alpine AS builder

WORKDIR /src
COPY . .

RUN go mod download && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "gosses"

FROM alpine:3.15.0

WORKDIR /

COPY --from=builder "/src/gosses" "/"

ENTRYPOINT ["/gosses"]
EXPOSE 8001
