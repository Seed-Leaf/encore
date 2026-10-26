# Acceptance checklist

Record evidence against the exact revision and target. An unchecked box means
pending or unverified, not a failed implementation. M1 local verification is recorded below. Required PR checks must also pass
before merge; M2–M4 are separate milestones.

## M0 investigation

- [x] Inspect baseline `bd202f849fe7cd13be9d11b31623f9abd03f06be`.
- [x] Distinguish CLI/daemon, application Docker export and the external frontend/platform.
- [x] Compile the native CLI with Go 1.27.3.
- [x] Cross-compile the static Linux/amd64 CLI with Zig musl; record source and binary identity.
- [x] Run non-root isolated Linux/amd64 daemon smoke; record empty `list-apps`, external-Origin 403 and unavailable-upstream 502.
- [x] Identify patched Go toolchain compatibility as separate from CLI compilation.

## M1 configuration and behavior

- [x] Variable absent preserves the local default.
- [x] Present empty string, empty array, invalid JSON and non-string entries fail.
- [x] HTTP, wildcard, credentials, path, query, fragment and malformed origins fail.
- [x] Canonical-equivalent duplicates fail; hostname case/default HTTPS port behavior is tested.
- [x] Configuration failure occurs before any daemon listener opens.
- [x] Exact configured HTTPS origins pass ordinary HTTP and WebSocket admission.
- [x] Non-allowlisted and malformed Origin values fail both paths.
- [x] Multiple Origin headers fail; cross-site requests fail except the documented no-Origin top-level navigation.
- [x] Missing Origin is allowed as documented, with remote restrictions still active.
- [x] Localhost does not bypass restrictions when remote mode is enabled.
- [x] `/__graphql` and object-content routes are denied without upstream access.
- [x] Remote UI requests permit GET/HEAD only after origin policy.
- [x] Only the eight documented read RPCs are admitted.
- [x] SQL, API calls, file/editor access, object operations and mutations are denied before execution.
- [x] An unknown/new RPC is denied by default.
- [x] Denied operations produce no state mutation or backend/proxy invocation.
- [x] Remote raw `process/output` notifications are suppressed; local notifications are preserved.
- [x] Focused package tests, formatting and CLI build pass.
- [x] Fork CI is configured for `develop` PRs/pushes and does not publish/deploy.
- [x] Independent review covers HTTP, WebSocket, RPC and notification paths.

## M2 package and browser acceptance

- [x] Pin dashboard runtime and Go builder images; define committed source and binary provenance.
- [ ] Pin patched Go/optional runtimes if application compilation is later added (omitted from this dashboard-only image).
- [ ] CLI version identifies the fork commit without spoofing an upstream release.
- [ ] Build and smoke-test the selected Linux image; retain its digest and source identity.
- [ ] Verify any target Go application build independently from the CLI.
- [x] Define state/cache persistence, permissions and backup/restore scope in the deployment contract.
- [ ] Select retention/backup scheduling before storing real application data.
- [ ] Use no production Docker socket, unrelated workspace mounts or ambient credentials.
- [x] Define Dokploy-owned runtime variables and explicit scope; no application secrets are needed.
- [ ] Verify browser read views with the current allowlist; list unsupported screens.
- [x] Identify hosted UI Cloud selection and domain-bound authentication as incompatible with the custom hostname.
- [x] Select a purpose-built embedded read-only UI; do not claim all daemon code paths are air-gapped.
- [ ] Verify remote HTML/CSS/JavaScript are embedded and no asset or Cloud auth upstream is contacted.
- [x] Verify owned HTML/JavaScript health markers; unrelated HTML and missing assets fail health checks (isolated asset fixtures).
- [ ] Verify same-origin protocol-aware WebSocket connections, without relying on a CSP URL upgrade.
- [ ] Verify app/status/trace read views, useful empty states, request errors and reconnection.
- [ ] Verify existing local mode retains its original frontend and API behavior.
- [ ] Remove obsolete remote frontend-upstream configuration from the target's Dokploy settings.
- [ ] Test deep links, static assets and WebSocket reconnect through real HTTPS/SSO.
- [ ] Verify authorized account access and an available authorized negative test case.
- [ ] Use only isolated development data; document shared visibility of all workspace apps/traces.
- [ ] Verify logout and expired sessions cannot open new protected content or WebSockets.
- [ ] Decide and verify the policy for already-open WebSockets after logout/session expiration.
- [ ] Verify anonymous and spoofed-header requests do not obtain protected content.
- [ ] Verify exact authorization policy rather than inferring it from the provider audience.
- [ ] Verify all alternate domains/direct ports and keep unrelated public hosts unchanged.

## M3 staged deployment

- [ ] Bind exact target hostname, service, environment, server, image and network from current state.
- [ ] Confirm authorization covers the selected staging deployment and ingress change.
- [ ] Verify the compatible existing auth proxy/provider before changing target routing.
- [ ] Capture private before-state and rollback instructions without repository secrets or private IDs.
- [ ] Verify authentication and authorization cover the entire dashboard listener.
- [ ] Keep the origin port private; do not trust client-provided identity headers.
- [ ] Confirm the tested digest is running and health/readiness reports the intended revision.
- [ ] Read back persisted settings and generated middleware/router order.
- [ ] Explicitly restart/redeploy to apply changed origins; do not assume source-build hash checks restart the daemon.
- [ ] Repeat anonymous, negative-identity and alternate-ingress checks after activation.
- [ ] Exercise rollback by preserving access restrictions, not disabling authentication.
- [ ] Reconcile any uncertain deployment result before retrying.

## M4 design gate

- [ ] Approve deployed-app metadata/trace identity, transport and authorization contracts.
- [ ] Define data scope, redaction, retention and stale/partial-data presentation.
- [ ] Bound collector credentials and capabilities if a collector is needed.
- [ ] Preserve current application workflow, schema and publication ownership.
- [ ] Describe observed deployed-app support only after end-to-end evidence exists.

## M1 local verification — 2026-10-26

The following checks passed on the proposed M1 working tree before the initial
commit. GitHub rebuilds the committed PR revision independently; use its current
Checks status rather than treating this record as a future release attestation.

```sh
go test -mod=readonly -race ./cli/daemon/dash/... ./cli/cmd/encore/daemon ./internal/env ./internal/version ./pkg/httpx
go build -mod=readonly -o /tmp/encore-dashboard-m1/encore ./cli/cmd/encore
```

Formatting, `git diff --check`, documentation links and native CLI `--help` also
passed. The Linux/amd64 CLI cross-build used cgo with Zig's musl target and static
linking. In an isolated non-root scratch container (amd64 emulation on aarch64):

- Allowed-origin WebSocket `version` succeeded in package tests; the container
  also returned `version` over the no-Origin WebSocket path.
- `traces/clear` returned JSON-RPC `-32601` without a backend mutation.
- Cloud GraphQL and object-content routes returned 403.
- An unlisted origin returned 403; POST to an asset path returned 405.
- An allowed origin reached the deliberately unavailable test asset upstream
  and returned 502. This is origin-policy evidence, not UI acceptance.
- The daemon stopped with SIGINT and exit 0; the test container was removed.

Independent code review covered HTTP, WebSocket, RPC, notification and asset-proxy
boundaries. These checks do not verify SSO, the browser UI, a pinned release image,
real application compilation, deployment, or production-data handling.

## Hosted UI deployment finding

The packaging follow-up passed HTTP/asset-shell checks and added CSP upgrading
for the hosted frontend's legacy URLs. Subsequent browser verification on the
protected custom hostname remained at a loading spinner: the hosted bundle
selected the Cloud application and attempted authentication bound to the
upstream domain. Those earlier checks did not establish a usable dashboard.

The embedded UI follow-up must pass the M2 checks above against its own revision.
Preserve the earlier M0/M1/proxy checks as historical evidence; a successful old
HTML healthcheck is not acceptance evidence for this replacement.

## Embedded UI package checks — 2026-10-26

Shell linting, Compose configuration validation, documentation links and whitespace
checks passed for the proposed replacement. The health script was run against
the actual UI files in isolated non-root Alpine containers with a read-only root
filesystem, no external network and no published ports. The owned HTML and both
JavaScript modules passed. A generic hosted HTML shell, a missing `app.js`, and
a missing `rpc.js` each failed the health check. These are health-script fixture
checks, not proof of compiled asset embedding, browser execution or SSO acceptance.
