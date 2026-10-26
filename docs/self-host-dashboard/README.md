# Self-hosted dashboard foundation

Status: M0 investigation and M1 boundary implementation complete (PR #1 merged).
M2 packaging and HTTPS compatibility are implemented in the deployment follow-up;
real browser/SSO acceptance and staged activation are tracked separately. M4 is pending.
Baseline: `bd202f849fe7cd13be9d11b31623f9abd03f06be`.

The goal is a small, internally accessible, read-only dashboard foundation that
can eventually run behind an authenticated Dokploy ingress. Start with the
existing development daemon and its local application/trace model. Extend the
boundary deliberately before adding deployed application discovery or operations.

This work does not recreate Encore Cloud. It does not establish a standalone,
air-gapped UI, production readiness, or a deployment to an existing service.
The current dashboard still proxies its frontend assets from another server;
compatibility of that UI with the restricted API is a separate milestone.

## Design and execution

| Document | Purpose |
| --- | --- |
| [High-level architecture](architecture.md) | Components, trust boundaries and what remains external |
| [Low-level design](implementation.md) | Configuration, HTTP/WebSocket admission and RPC policy |
| [Roadmap](roadmap.md) | Milestones, deliverables and completion criteria |
| [Checklist](checklist.md) | Review, test, packaging and staged deployment gates |
| [Deployment](deployment.md) | Pinned image, state, health, Dokploy settings and rollback |

## Current scope and future scope

| Scope | State |
| --- | --- |
| Build the open-source CLI from the baseline source | Investigated and compiled; see M0 evidence below |
| Opt in to exact remote HTTPS origins | M1 implemented; canonical origin matching, not authentication |
| Restrict remote sessions to an explicit read-only RPC allowlist | M1 implemented; raw process output suppressed |
| Preserve default local development behavior when unset | M1 compatibility requirement |
| Authenticate and authorize users | External SSO proxy; integration acceptance pending |
| Browser UI, assets and behavior without Cloud dependencies | M2, unverified |
| Pinned dashboard-only package and Dokploy runtime configuration | M2 implemented; release-specific verification required |
| Staged deployment to an exact verified target | M3, not deployed |
| Observe separately deployed applications | M4, not implemented |
| Production control plane, provisioning, deployment automation or multi-tenant RBAC | Outside this foundation |

## M0 evidence and limits

The CLI was built natively with Go 1.27.3 and cross-compiled for Linux/amd64
using Zig's musl target, cgo and external static linking. The cross-build exited
successfully without compiler diagnostics. Its build metadata recorded the
baseline revision and `vcs.modified=false`; `file` identified a statically linked
x86-64 ELF executable.

The Linux binary's SHA-256 was
`ed24f425a1c18079c93bdead2d9f6f5ba381a24f6e1f8411c99860b462e065cd`.
This is an investigation artifact identity, not a release digest for this change.
The same binary was packaged in an isolated `scratch` image and run as UID 65532
on a Linux/aarch64 Docker host using amd64 platform emulation. The daemon started
with private state, and WebSocket `list-apps` returned an empty registry. An
external-Origin HTTP request returned 403 under the baseline local policy;
disabling the frontend upstream produced 502, confirming the missing standalone
UI. No credentials, host workspaces or Docker socket were mounted. The test
container was stopped and removed.

The smoke image omitted CA certificates, patched Go and SDK files. A release-check
TLS error was observed; the package was not suitable for an operational release.
Container exit 143 did not establish graceful application draining. Browser UI,
real app compilation, DB access, trace ingestion and Dokploy deployment were not
verified. These are baseline checks, not M1 acceptance evidence.

These checks establish CLI buildability only. Encore application compilation
uses its patched Go distribution and sets `GOTOOLCHAIN=local`; compiling the CLI
with ordinary Go 1.27 does not verify existing Go 1.27 application compatibility.
See [the compiler invocation](../../v2/compiler/build/build.go) and
[source-build instructions](../../CONTRIBUTING.md).

Existing application runtimes, Hatchet workflows, database migration ownership
and artifact storage contracts remain outside this change.

## Source map

| Area | Source |
| --- | --- |
| Daemon initialization and listeners | [daemon.go](../../cli/cmd/encore/daemon/daemon.go) |
| Dashboard HTTP and WebSocket routes | [server.go](../../cli/daemon/dash/server.go) |
| Remote origin and RPC allowlist policy | [access.go](../../cli/daemon/dash/access.go) |
| Dashboard JSON-RPC handlers | [dash.go](../../cli/daemon/dash/dash.go) |
| Frontend upstream and platform configuration | [conf.go](../../internal/conf/conf.go) |
| Cached frontend proxy | [dashproxy.go](../../cli/daemon/dash/dashproxy/dashproxy.go) |
| Platform GraphQL proxy | [apiproxy.go](../../cli/daemon/dash/apiproxy/apiproxy.go) |
| Application image export | [export.go](../../cli/daemon/export/export.go) |
| Fork validation workflow | [self-host-dashboard.yml](../../.github/workflows/self-host-dashboard.yml) |

Official references: [Encore self-hosted application builds](https://encore.dev/docs/self-host/build)
and [Dokploy application authentication](https://docs.dokploy.com/docs/core/enterprise/sso/application-authentication).
Self-hosting an application image and running the development dashboard are
different deployment units.
