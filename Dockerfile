# syntax=docker/dockerfile:1.7

# -------- build stage --------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Cache deps first.
COPY src/go.mod src/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Then the rest of the module.
COPY src/ ./

# Pure-Go build: modernc.org/sqlite has no CGo dependency, so we can target
# a fully static binary that runs on distroless/scratch.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/vexrss .

# -------- runtime stage --------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/vexrss /app/vexrss

# Persisted state lives here; mount a volume at /data in prod.
VOLUME ["/data"]
EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/vexrss"]
CMD ["-addr", ":8080", "-db", "/data/vexrss.db", "-poll", "15m"]
