#!/bin/sh
set -eu

page=$(mktemp /tmp/encore-dashboard-health.XXXXXX)
trap 'rm -f "$page"' EXIT HUP INT TERM

# Request through the actual dashboard proxy. A cached page is sufficient for
# liveness; warm a cold cache via the configured HTTPS upstream if needed.
# This does not assert browser/JavaScript, SSO or WebSocket acceptance.
if ! wget -q -T 4 -O "$page" --header 'Cache-Control: only-if-cached' \
    http://127.0.0.1:9400/ 2>/dev/null; then
    wget -q -T 4 -O "$page" http://127.0.0.1:9400/ 2>/dev/null
fi
grep -Eiq '<!doctype[[:space:]]+html|<html([[:space:]>])' "$page"
grep -Eiq '<script([[:space:]>])' "$page"
