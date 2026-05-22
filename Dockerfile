# --- stage 1: build ---------------------------------------------------------
# Pin Go version to match go.mod. modernc.org/sqlite is pure-Go, so we keep
# CGO_ENABLED=0 — that lets the final stage be `scratch` (no libc needed).
FROM golang:1.26.3-alpine AS build

# tzdata: zoneinfo db that scratch needs for time.LoadLocation
# (slog formatters happily use it). Without it the runtime falls back
# to UTC, which is fine but less friendly in logs.
RUN apk add --no-cache tzdata

# Build-time hooks. `VERSION` is baked into the binary via -ldflags so /about
# can show the release tag. Defaults to "dev" for unversioned local builds.
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

# Cache module downloads on a separate layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH

# -trimpath strips local paths; -s -w drop debug + symbol tables.
RUN go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/bot \
    ./cmd/bot

# --- stage 2: runtime -------------------------------------------------------
# `scratch` is ~0 bytes; the resulting image is just our binary + the TLS
# trust store + tz data (Telegram + osvita are both HTTPS, abit-poisk has
# a broken chain we already skip-verify in code).
FROM scratch

# CA certs — needed for HTTPS to osvita.ua and Telegram.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Timezone data — slog timestamps look saner with a real zoneinfo.
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

# The bot writes ./data/<...> by default; the volume mount keeps it across
# restarts. WORKDIR makes the relative path land in /data.
WORKDIR /data

COPY --from=build /out/bot /bot

# Document the only thing we expect the operator to mount.
VOLUME ["/data"]

# Use a non-root UID — `scratch` has no /etc/passwd, but Docker just stores
# the numeric id, which is enough to drop root privileges.
USER 65532:65532

ENTRYPOINT ["/bot"]
