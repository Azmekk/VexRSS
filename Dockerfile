# syntax=docker/dockerfile:1.7

# -------- build stage --------
FROM golang:1.26-bookworm AS build
# Allow Go to auto-fetch a newer toolchain if go.mod declares one past the
# image's baseline.
ENV GOTOOLCHAIN=auto

WORKDIR /src

# Cache deps first.
COPY src/go.mod src/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Then the rest of the module.
COPY src/ ./

# Pure-Go build (no CGo) so the runtime image can be tiny and static.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/vexrss .

# -------- runtime stage --------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates su-exec tini \
    && addgroup -S -g 1000 vexrss \
    && adduser -S -u 1000 -G vexrss -H vexrss

WORKDIR /app
COPY --from=build /out/vexrss /app/vexrss
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Persisted state lives here; mount a volume or bind at /data in prod.
VOLUME ["/data"]
EXPOSE 8080

# tini reaps zombies and forwards signals so `docker stop` shuts down cleanly.
# The entrypoint fixes /data ownership, then drops privileges to the vexrss
# user before exec'ing the binary.
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["-addr", ":8080", "-db", "/data/vexrss.db", "-poll", "15m"]
