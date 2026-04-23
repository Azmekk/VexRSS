#!/bin/sh
set -e

# Fix permissions on the data directory so SQLite can open the DB.
# Host bind mounts arrive owned by the host user (often uid 1000), which
# the non-root vexrss user can't write to until we chown it. This requires
# the container to start as root; we drop privileges immediately after.
if [ "$(id -u)" = "0" ]; then
  mkdir -p /data
  chown -R vexrss:vexrss /data
  exec su-exec vexrss:vexrss /app/vexrss "$@"
fi

# Caller pinned a non-root user via `docker run --user ...`; assume they
# know what they're doing and skip the chown.
exec /app/vexrss "$@"
