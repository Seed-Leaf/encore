# Roadmap

Milestones are gates, not promised dates. A successful CLI build does not complete
the application, UI, ingress or deployment gates that follow it.

| Milestone | State | Deliverable | Exit criteria |
| --- | --- | --- | --- |
| M0 — inspect and build | Complete for source/build investigation | Source map, build/deploy unit distinction, native/Linux CLI and isolated container smoke evidence | Record the exact source/toolchain/binary identities and limits of the checks |
| M1 — restrict remote access | Merged in PR #1; CI passed | Strict opt-in origins, uniform HTTP/WS policy, read-only RPC allowlist, docs and focused CI | Required tests pass; independent review finds no unguarded dispatch path; local default stays compatible |
| M2 — package and verify UI | Packaging implemented; replacing incompatible hosted UI with embedded read-only UI | Pinned image/package and UI, browser acceptance, SSO design, Dokploy config contract | Useful and empty read views work without Cloud UI/auth; auth and alternate-ingress tests pass in an isolated environment |
| M3 — staged Dokploy deployment | First target activated; UI repair and release acceptance pending | One exact development target behind verified ingress | Authorized target bound, image verified, useful UI and SSO/bypass checks pass, rollback evidence retained |
| M4 — deployed-app registry | Future | Metadata/trace identity and ingestion design, optional collector | Scope and data/identity contracts reviewed before implementation; no claim of automatic fleet discovery |

## M1: boundary before exposure

Implement configuration parsing before daemon listeners, one shared origin policy,
HTTP route/method restrictions, suppression of
raw process output notifications, and a dispatcher-level allowlist. Add regression
tests for invalid startup configuration, disallowed origins and operations, and
default local behavior. Tests must prove that denied requests do not reach a
side-effecting handler or proxy.

M1 also scrubbed credentials from the original remote asset proxy. The embedded
UI follow-up removes that upstream from the remote serving path while preserving
the request/RPC boundary and original local mode.

The fork's [focused workflow](../../.github/workflows/self-host-dashboard.yml)
runs on PRs into `develop` and pushes to `develop`, using Ubuntu 24.04 for
formatting, focused race tests and CLI build/help. Its actual results must be
recorded before marking M1 verified. It does not rely on the upstream workflow's
`main`/runner assumptions. No CI step publishes or deploys M1.

## M2: browser, identity and reproducible package

[Deployment packaging](deployment.md) defines a dashboard-only image. Application
compilation toolchains are explicitly omitted; testing application builds remains
a separate scope. The first hosted frontend selected its Cloud application for
the custom hostname and stalled on Cloud authentication. Upgrading legacy URLs
with CSP did not resolve that cause.

Ship the fork's own read-only HTML/CSS/JavaScript in the CLI. Keep all assets and
the protocol-aware WebSocket on the protected origin, and preserve the strict
RPC allowlist. Test app/status/trace views, useful zero-app/zero-trace states,
visible failures and reconnect behavior. Verify remote requests never reach the
hosted frontend or platform authentication; leave original local mode intact.
An embedded UI is not a deployed-app ingestion system or an offline guarantee
for every daemon feature.

Verify real HTTPS sign-in and authorization, fresh anonymous requests, a permitted
negative identity case, deep links, static assets, WebSocket connection/reconnection,
logout/session expiration and spoofed identity headers. Forward auth checks only
the handshake: explicitly decide and verify how already open WebSockets expire
or are revoked. Protect the entire listener.
Keep direct container ports and alternate unprotected domains inaccessible.

Use isolated development data only; all admitted users currently share workspace
visibility. Build a Linux package from the pinned CLI/UI source and builder inputs;
record architecture and image digest. Store runtime configuration/secrets in Dokploy, with explicit
service references. Define private persistent daemon/cache paths, restart behavior
and least-privilege mounts. Test the chosen Go application toolchain separately
from CLI compilation. Define readiness and source-revision probes appropriate to
this container rather than copying another application's health command.

## M3: one staged deployment

Resolve the exact hostname, application/service, environment, server, image,
network and ingress from current state. Verify the existing identity provider and
proxy instead of copying historic IDs or provisioning a second provider by default.
Do not mutate unrelated public hosts, application routes or shared auth settings.

Capture private before-state and a concrete rollback plan. Deploy only after the
target and gate are verified and the selected scope is authorized. Confirm the
running image, persisted settings and generated route/middleware order. A green
container status alone does not prove authentication or a useful dashboard.

## M4: remote applications are a separate system

Define app/environment/build identities, registration, trace transport and
authorization before adding an endpoint. Choose how to handle stale registrations,
partial telemetry, retention and schema evolution. An optional collector should
have a narrow push or pull contract and separate credentials. It must not grant
the dashboard deployment or database authority.

Do not reuse local daemon process assumptions as remote execution authority.
Existing Hatchet workflow scheduling, paid-work fences, database migrations and
artifact verification remain in their current owners unless a later task changes
those contracts explicitly.
