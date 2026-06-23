FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY ./entrypoint.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /entrypoint entrypoint.go

FROM alpine:latest

WORKDIR /iperf

RUN apk add --no-cache --no-check-certificate iperf3
COPY --from=builder /entrypoint ./entrypoint

ENV PORT="5201"

ENTRYPOINT ["./entrypoint"]
