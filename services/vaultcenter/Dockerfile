ARG DOCKER_MIRROR=
FROM ${DOCKER_MIRROR}golang:1.24-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-vaultcenter ./cmd/main.go

ARG DOCKER_MIRROR=
FROM ${DOCKER_MIRROR}alpine:3.21
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /build/veilkey-vaultcenter /usr/local/bin/
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
VOLUME /data
EXPOSE 10180
ENTRYPOINT ["docker-entrypoint.sh"]
