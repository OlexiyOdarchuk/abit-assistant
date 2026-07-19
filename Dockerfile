# Single image that runs BOTH the Telegram bot and the web server in one
# process (cmd/app) — for hosts that allow only one Dockerfile per repo.
#
# The whole app is pure Go (pgx included), so CGO_ENABLED=0 lets the final
# image be `scratch`.
#
# --- stage 1: build ---------------------------------------------------------
FROM golang:1.26.3-alpine AS build

# tzdata: zoneinfo db that scratch needs for time.LoadLocation.
RUN apk add --no-cache tzdata

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

# cmd/app embeds internal/web/dist (the built SPA is committed to the repo),
# so no Node/npm step is needed here.
RUN go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/app \
    ./cmd/app

# --- stage 2: runtime -------------------------------------------------------
# `scratch` is ~0 bytes; the image is just the binary + TLS trust store + tz.
# State lives in PostgreSQL (DATABASE_URL), so there is nothing to mount.
FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/app /app

# Non-root UID — scratch has no /etc/passwd, but the numeric id drops root.
USER 65532:65532

# Web listens here; the platform routes the domain in. The bot uses
# long-polling (no inbound port).
ENV HTTP_ADDR=:8080
EXPOSE 8080

# Required env: DATABASE_URL (postgres://…), TELEGRAM_TOKEN.
# Optional: ADMIN_IDS, LOG_LEVEL. Without TELEGRAM_TOKEN only the web runs.
ENTRYPOINT ["/app"]
