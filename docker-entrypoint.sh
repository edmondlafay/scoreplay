#!/bin/sh
set -e
# Named volumes are owned by root when Docker mounts them over an image layer.
# Fix ownership on each start so the non-root user can write to uploads/.
chown scoreplay:scoreplay /app/uploads
exec su-exec scoreplay "$@"
