ARG DOCKER_MIRROR=
FROM ${DOCKER_MIRROR}golang:1.24 AS builder
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-vaultcenter ./cmd/main.go

ARG DOCKER_MIRROR=
FROM ${DOCKER_MIRROR}alpine:3.21
RUN apk add --no-cache ca-certificates curl gcompat
COPY --from=builder /build/veilkey-vaultcenter /usr/local/bin/
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
VOLUME /data
EXPOSE 10180
ENTRYPOINT ["docker-entrypoint.sh"]
