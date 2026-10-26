#!/bin/sh
set -eu

page=$(mktemp /tmp/encore-dashboard-health.XXXXXX)
script=$(mktemp /tmp/encore-dashboard-script.XXXXXX)
trap 'rm -f "$page" "$script"' EXIT HUP INT TERM

# Require the owned embedded UI and its JavaScript, not an arbitrary HTML shell
# from a hosted dashboard. All requests stay on the local listener; there is
# no asset upstream/cache dependency. Browser, SSO and RPC checks remain separate.
wget -q -T 3 -O "$page" http://127.0.0.1:9400/ 2>/dev/null
grep -Fq '<meta name="encore-dashboard" content="remote-read-only">' "$page"
wget -q -T 3 -O "$script" http://127.0.0.1:9400/app.js 2>/dev/null
grep -Fq 'encore-remote-read-only-ui' "$script"
wget -q -T 3 -O "$script" http://127.0.0.1:9400/rpc.js 2>/dev/null
grep -Fq 'export class DashboardRPC' "$script"
