#!/bin/sh

PUID=${PUID:-1000}
PGID=${PGID:-1000}

# Resolve the actual group name for PGID, or create "appgroup" if none exists.
GROUP_NAME=$(getent group "$PGID" | cut -d: -f1)
if [ -z "$GROUP_NAME" ]; then
  GROUP_NAME="appgroup"
  addgroup -g "$PGID" "$GROUP_NAME"
fi

# Create the user if it doesn't already exist.
if ! getent passwd "$PUID" > /dev/null 2>&1; then
  adduser -u "$PUID" -G "$GROUP_NAME" -D -h /data appuser
fi

chown -R "$PUID:$PGID" /data

exec su-exec "$PUID:$PGID" /server "$@"
