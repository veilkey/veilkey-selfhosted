ARG DOCKER_MIRROR=
FROM ${DOCKER_MIRROR}golang:1.24-bookworm AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-vaultcenter ./cmd/main.go

ARG DOCKER_MIRROR=
FROM ${DOCKER_MIRROR}debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*
COPY --from=builder /build/veilkey-vaultcenter /usr/local/bin/
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
VOLUME /data
EXPOSE 10181
ENTRYPOINT ["docker-entrypoint.sh"]
