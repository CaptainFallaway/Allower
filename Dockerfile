FROM golang:1.26-alpine AS deps

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY go.* .
RUN go mod download

COPY ./pkg ./pkg
COPY ./internal ./internal

FROM deps AS lookup_build

COPY ./cmd/lookup ./cmd/lookup
RUN CGO_ENABLED=0 GOOS=linux go build -o ./bin/lookup -ldflags="-w -s" ./cmd/lookup

FROM deps AS allower_build

COPY ./cmd/allower ./cmd/allower
RUN CGO_ENABLED=0 GOOS=linux go build -o ./bin/allower -ldflags="-w -s" ./cmd/allower

FROM scratch AS final

COPY --from=lookup_build /app/bin/lookup /lookup
COPY --from=allower_build /app/bin/allower /allower

COPY --from=deps /etc/ssl/certs/ca-certificates.crt /certs/ca-certificates.crt
ENV SSL_CERT_FILE=/certs/ca-certificates.crt

WORKDIR /persist # This is where the config and data will be stored. It should be mounted as a volume.
WORKDIR /

VOLUME /persist

ENV IPINFO_DIR="/persist"
ENV IPINFO_SYNC="12h"
ENV PRETTY_LOGGING="false"
ENV DEBUG="false"
ENV CONFIG_PATH="/persist/config.yaml"

ENTRYPOINT ["/allower"]
