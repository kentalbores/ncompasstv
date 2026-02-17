# ==============================================================================
# n-compasstv: Multi-stage Dockerfile
# Stage 1: Build the Go binary with CGO (for libVLC bindings)
# Stage 2: Minimal runtime with VLC libraries
# ==============================================================================

# ---------- Stage 1: Build ----------
FROM golang:1.22-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvlc-dev \
    vlc-plugin-base \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=docker-dev
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=1 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o /out/n-compasstv ./cmd/player

# ---------- Stage 2: Runtime ----------
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvlc5 \
    vlc-plugin-base \
    vlc-plugin-video-output \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --system --create-home --shell /bin/false ncompass \
    && usermod -aG video,render,audio ncompass

RUN mkdir -p /playlist /etc/n-compasstv /var/log/n-compasstv \
    && chown -R ncompass:ncompass /playlist /var/log/n-compasstv

COPY --from=builder /out/n-compasstv /usr/local/bin/n-compasstv
RUN chmod +x /usr/local/bin/n-compasstv

RUN echo '{"id":"","key":"","name":"n-compasstv","endpoint":"","heartbeat_interval_sec":60}' \
    > /etc/n-compasstv/config.json

VOLUME ["/playlist"]

USER ncompass

ENTRYPOINT ["n-compasstv"]
CMD ["run", "--playlist", "/playlist", "--config", "/etc/n-compasstv/config.json"]
