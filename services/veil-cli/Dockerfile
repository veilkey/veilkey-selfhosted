FROM rust:1-alpine AS builder
RUN apk add --no-cache musl-dev openssl-dev openssl-libs-static perl
WORKDIR /build

COPY veil-cli/ ./veil-cli/
WORKDIR /build/veil-cli
RUN cargo build --release

FROM alpine:3.20
RUN apk add --no-cache bash curl

ARG BIN_DIR=/usr/local/bin
COPY --from=builder /build/veil-cli/target/release/veil ${BIN_DIR}/veil
COPY --from=builder /build/veil-cli/target/release/veilkey-cli ${BIN_DIR}/veilkey-cli
COPY --from=builder /build/veil-cli/target/release/veilkey-session-config ${BIN_DIR}/veilkey-session-config
COPY veil-cli/examples/.veilkey.yml /root/.veilkey.yml

WORKDIR /root
ENTRYPOINT ["/bin/bash"]
