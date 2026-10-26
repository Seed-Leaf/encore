# Dashboard image and Dokploy deployment

This package runs the M1 read-only development dashboard in one container. It is
an internal evaluation service, not a production application control plane. A
fresh instance has an empty application registry. It does not discover separately
deployed applications or collect their traces.

The [Dockerfile](../../deploy/dashboard/Dockerfile) packages the source-built CLI,
Mozilla CA certificates and small startup/health scripts. It deliberately omits
the patched Encore Go toolchain, application runtime sources, Node, supervisor
and Docker client. Those inputs are unnecessary for serving existing dashboard
metadata; compiling or running applications in this image is unsupported.

## Image inputs and identity

The Dockerfile pins the official Alpine 3.23 and Go 1.26 image indexes by digest.
The resolved images were Alpine 3.23.7 and Go 1.26.10 on 2026-10-26. The pinned
Alpine image includes `ca-certificates-bundle`; no runtime package installation
is needed. Refresh these pins deliberately when accepting security updates.

There are two targets:

| Target | Input | Intended use |
| --- | --- | --- |
| `source` (default) | A clean fork source revision; Go builder with cgo and static linking | Build on the target Linux architecture |
| `prebuilt` | A verified static Linux CLI named `encore` in the `dashboard_binary` build context | Package a native or Zig cross-build without emulated Go compilation |

Set `SOURCE_REVISION` to the full commit used for the CLI. The source target
embeds `v0.0.0-develop+<commit>` as its version. The prebuilt target labels the
image with this revision but cannot independently establish binary provenance:
verify the binary's build metadata and SHA-256 before packaging. Never reuse the
baseline investigation binary to deploy code from a later commit.

Build from a clean committed snapshot. For example, after merging the packaging
change, from the repository root:

```sh
SOURCE_REVISION=$(git rev-parse HEAD)
build_context=$(mktemp -d)
git archive "$SOURCE_REVISION" | tar -x -C "$build_context"
docker buildx build --platform linux/amd64 --target source \
  --build-arg SOURCE_REVISION="$SOURCE_REVISION" \
  -f "$build_context/deploy/dashboard/Dockerfile" \
  -t encore-dashboard:review --load "$build_context"
```

The source builder installs Alpine's C build tools. Its package mirror and the
Go module download service are build-time dependencies. Base-image pins and
`go.sum` constrain inputs, but this is not a claim of bit-for-bit reproducibility
across changes to the package mirror. Record compiler/package versions and
build provenance, then deploy the resulting immutable image digest.

For an existing verified cross-build, the named context must contain only the
selected binary, named `encore`. Do not place a home directory or credentials
in that context:

```sh
SOURCE_REVISION=$(git rev-parse HEAD)
binary_context=$(mktemp -d)
cp /path/to/verified/encore-linux-amd64 "$binary_context/encore"
go version -m "$binary_context/encore"
shasum -a 256 "$binary_context/encore"
docker buildx build --platform linux/amd64 --target prebuilt \
  --build-context dashboard_binary="$binary_context" \
  --build-arg SOURCE_REVISION="$SOURCE_REVISION" \
  -f deploy/dashboard/Dockerfile -t encore-dashboard:review --load .
```

The binary must be Linux/amd64 for these examples and statically linked. Building
the CLI on macOS produces a different executable format. For a Zig cross-build,
use `GOOS=linux`, `GOARCH=amd64`, `CGO_ENABLED=1`,
`CC='zig cc -target x86_64-linux-musl'` and
`-ldflags='-linkmode external -extldflags -static'`. Record the actual Go and Zig
versions. Validate the resulting ELF and source revision before using the
prebuilt target. Neither target packages credentials or an Encore account token.

## Runtime contract

The [entrypoint](../../deploy/dashboard/entrypoint.sh) requires
`ENCORE_DEVDASH_REMOTE_ORIGINS` and refuses to start when it is absent or empty.
The daemon validates the remaining JSON/origin syntax before opening listeners.
The image cannot silently fall back to the more capable local mode.

The entrypoint executes `encore daemon --foreground` as UID/GID 65532. It fixes
the dashboard listener at `0.0.0.0:9400`; other TCP listeners remain on loopback
inside the container. The local management socket remains inside its state
directory. CLI telemetry is disabled, `ENCORE_AUTH_KEY` is rejected, and no
application credentials should be supplied. Console logs go to stderr;
the daemon's duplicate log file goes to `/dev/null` to avoid unbounded log files.

| Path | Purpose | Persistence |
| --- | --- | --- |
| `/state/config/encore/encore.db` | Daemon registry, metadata, onboarding and trace database | Named volume; sensitive even in read-only serving mode |
| `/state/config` | Other CLI configuration | Same named volume; do not import machine account credentials |
| `/state/cache` | Local daemon socket and frontend/cache data | Same named volume; socket is recreated at startup |
| `/state/data` | Reserved local daemon data | Same named volume; no application runtime use in this package |
| `/state/home` | Nonroot user's home | Same named volume |
| `/tmp` | Healthcheck response file and temporary files | Writable temporary storage; optional tmpfs |

The image owns `/state` as 65532:65532. A new Docker named volume is initialized
from that directory; verify ownership on the actual deployment. An existing
volume with incompatible permissions must be repaired through the scoped
deployment procedure, not by running the service permanently as root.

Run exactly one replica with **stop-first updates and rollbacks**. Multiple
daemons must not share this volume: each can replace the management socket, and
the SQLite database is not a shared control-plane database. Stop the daemon
before taking a filesystem backup of its database and WAL files; preserve that
backup before a revision that changes its management schema. This does not run
or replace application database migrations.

The daemon currently handles `SIGINT` in
[runMain](../../cli/cmd/encore/daemon/daemon.go). Both the image and reference
Compose configuration select `SIGINT`, with a 30-second stop allowance in
Compose. Verify the actual platform honors this setting. This gives the daemon
its cleanup path; it is not evidence of graceful draining for application jobs.

## UI availability and health

`ENCORE_DEVDASH_URL` defaults to `https://devdash.encore.dev`. The daemon proxies
the actual frontend HTML, JavaScript and assets over HTTPS using the image's
CA bundle. The UI is not bundled or pinned with the CLI, and the external asset
service remains a runtime dependency. Only use a trusted reviewed replacement
if changing this URL: its JavaScript executes under the dashboard's origin.

The [healthcheck](../../deploy/dashboard/healthcheck.sh) requests `/` through the
local dashboard listener and requires HTML containing a script element. It first
requests cached content and fetches through the configured upstream on a cache
miss. Thus a warm cache can keep the process healthy during an upstream outage;
this intentionally does not monitor continuous upstream freshness. Normal
browser requests still use the proxy's cache policy.

A healthy container proves the local HTTP path can serve a UI shell. It does not
prove JavaScript execution, WebSocket RPC compatibility, SSO authorization or
deployed application visibility. Before accepting a release, test the actual
browser page and assets, `version`/`list-apps` over WebSocket, blocked mutation
RPCs and deep links through the protected hostname. Keep evaluation data free of
production payloads. All admitted users can see the whole daemon workspace;
there is no per-application or tenant authorization.

## Dokploy Application configuration

Use the Docker image provider with the pushed `repository@sha256:<digest>`.
Dokploy is the authoritative runtime configuration source. Configure reusable
values at project scope, environment-specific values at environment scope and
service-only values on this application. Explicitly reference only values this
service needs. Do not introduce a manually managed host `.env` file.

| Setting | Value or requirement |
| --- | --- |
| Image | Verified immutable digest for the selected Linux architecture |
| Start command | Image entrypoint; no command override |
| Remote origins | `ENCORE_DEVDASH_REMOTE_ORIGINS=["https://dashboard.example.com"]`, replacing the reserved example with the exact bound hostname |
| UI upstream | Optional `ENCORE_DEVDASH_URL`; default is the public HTTPS asset service |
| Persistent mount | Dedicated named volume at `/state`; do not share it with another daemon |
| Replicas/update policy | One replica; stop-first update and rollback |
| Domain target | Container port 9400 on the private Dokploy ingress network |
| Published host ports | None |
| Extra mounts | No Docker socket, developer home, application workspace or credentials |
| Health | Preserve the image healthcheck; writable `/tmp` is required |
| Logs | Use platform log limits/rotation; inspect without dumping tokens or trace payloads |

Bind the exact application, project/environment, server, hostname and domain
before changing routing. Configure external SSO with exact authorized email-domain
policy over the entire listener, including HTML, assets, WebSocket upgrades and
deep links. Validate aliases and alternate ingress; a missing Origin is allowed
by the daemon and is not an authentication bypass defense. Do not trust a
client-supplied identity header. Preserve public services and shared identity
infrastructure.

The native Application configuration used for this deployment may not expose
root-filesystem read-only, capability-drop or `no-new-privileges` fields. Do not
claim those controls are active from the image alone. The
[reference Compose file](../../deploy/dashboard/compose.yaml) includes those
additional controls, a `/tmp` tmpfs, log rotation and a memory limit when using a
Compose deployment instead. It intentionally publishes no host port and creates
no ingress router or authentication middleware. Supply the existing private
ingress network name and add the protected route through Dokploy. Resource
limits are starting settings, not a measured capacity guarantee.

Saving environment variables does not update the process. Redeploy the selected
image digest after changing origins or other configuration, then verify the
resolved state. Source/develop builds do not guarantee automatic daemon restart
when their configuration hash changes.

## Release and rollback evidence

Before exposing this service, record the image/source/binary identities, volume
binding, single-replica update policy, listener reachability and protected router
configuration. Check fresh unauthenticated access, spoofed identity headers,
authorized browser sign-in, an available unauthorized identity, assets, a deep
link and WebSocket traffic. Document any test that could not be completed.
SSO at the WebSocket handshake does not revoke an already-open socket on logout;
agree and verify a session lifetime/reconnect policy before wider use.

If access protection or browser acceptance fails, keep ingress closed or restore
the previous protected route. Stop the target, restore the prior image and
compatible state snapshot if needed, and verify the gate before reopening.
Never remove `ENCORE_DEVDASH_REMOTE_ORIGINS` while retaining remote exposure.
Do not undo shared SSO infrastructure to roll back this service.

The broader [milestone checklist](checklist.md) remains authoritative for M2/M3
acceptance. A successful image build or HTTP healthcheck alone does not complete
those milestones.
