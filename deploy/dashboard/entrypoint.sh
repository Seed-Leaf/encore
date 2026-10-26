#!/bin/sh
set -eu

# The CLI's unset default is deliberately local and more capable. This image is
# only for remote read-only serving, so missing configuration must never select it.
if [ -z "${ENCORE_DEVDASH_REMOTE_ORIGINS:-}" ]; then
    echo "ENCORE_DEVDASH_REMOTE_ORIGINS is required for this dashboard image" >&2
    exit 1
fi
if [ "$#" -ne 0 ]; then
    echo "This image only runs the foreground read-only dashboard daemon" >&2
    exit 1
fi

# Keep local management/debug/runtime listeners inside the container. These
# fixed paths also match the image healthcheck and the documented state mount.
export HOME=/state/home
export XDG_CONFIG_HOME=/state/config
export XDG_CACHE_HOME=/state/cache
export ENCORE_CONFIG_DIR=/state/config/encore
export ENCORE_CACHE_DIR=/state/cache/encore/cache
export ENCORE_DATA_DIR=/state/data
export ENCORE_DAEMON_LOG_PATH=/dev/null
export ENCORE_DEVDASH_LISTEN_ADDR=0.0.0.0:9400
export ENCORE_MCPSSE_LISTEN_ADDR=127.0.0.1:9900
export ENCORE_OBJECTSTORAGE_LISTEN_ADDR=127.0.0.1:9800
export DISABLE_ENCORE_TELEMETRY=1

if [ -n "${ENCORE_AUTH_KEY:-}" ]; then
    echo "ENCORE_AUTH_KEY must not be provided to the read-only dashboard image" >&2
    exit 1
fi

umask 077
mkdir -p "$ENCORE_CONFIG_DIR" "$ENCORE_CACHE_DIR" "$ENCORE_DATA_DIR" "$HOME"
if [ ! -w "$ENCORE_CONFIG_DIR" ] || [ ! -w "$XDG_CACHE_HOME" ]; then
    echo "The /state volume must be writable by UID/GID 65532" >&2
    exit 1
fi

# Encore performs strict JSON/origin validation before opening its listeners.
# Exec makes the daemon PID 1 so the image's SIGINT reaches it directly.
exec /usr/local/bin/encore daemon --foreground
