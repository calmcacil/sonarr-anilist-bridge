#!/bin/sh

PUID=${PUID:-1000}
PGID=${PGID:-1000}

getent group "$PGID" > /dev/null 2>&1 || addgroup -g "$PGID" appgroup

getent passwd "$PUID" > /dev/null 2>&1 || \
  adduser -u "$PUID" -G appgroup -D -h /data appuser

chown -R "$PUID:$PGID" /data

exec su-exec "$PUID:$PGID" /server "$@"
