# Web acceptance — 2026-09-28

## Scope and environment

Hermes upstream remains `v2026.9.14`, unchanged custom agent image digest
`sha256:feecb5d71f4876758b61e83cf1527fa6cdbb4f7e8919808c5eed7daf5085640f`.

Cluster: `berger-apps`, Kubernetes `v1.34.7+k3s1`, Gateway API v1.5.1,
Istio v1.30.3. Existing `hermes-operator-test/hermes-smoke` reused its PVC;
no reset/onboarding wipe was performed. A temporary `hermes-web-smoke` had
independent credentials/PVC and no Telegram token. Its model credential reused
the dedicated smoke inference credential; this is a test fixture, not a shared
production-key recommendation.

Infrastructure changes were explicitly authorized. Cloudflare DNS-only A
records `agents.s2technologies.ru` and `*.agents.s2technologies.ru` now point
to `94.228.243.175`. [Platform MR 126](https://gitlab.s2technologies.ru/s2technologiesru/infra/platform/argocd-bootstrap/-/merge_requests/126)
adds `*.agents.s2technologies.ru` to the existing wildcard certificate and
Gateway listener hosts through GitOps. Its CI and all three cluster renders
passed. Certificate revision 3 reached Ready; both names passed HTTPS
certificate validation without bypass. The existing external Gateway namespace
attachment label was added only to the dedicated test namespace after confirming
it contained no HTTPRoutes and only the smoke installation.

## Executed evidence

| Check | Result |
| --- | --- |
| Existing Telegram-only CR after operator upgrade | Ready, Web disabled; same PVC. |
| Primary smoke Telegram + Path Web | Ready, `https://agents.s2technologies.ru/hermes-smoke`. |
| Independent Web-only Subdomain | Ready, `https://hermes-web-smoke.agents.s2technologies.ru`. |
| Primary smoke Path → Subdomain | Ready, `https://hermes-smoke.agents.s2technologies.ru`. |
| Actual HTTPS on both routing modes | Correct certificate; no TLS verification bypass. |
| Native guard | `/api/status`: auth required, basic provider. |
| Anonymous API / wrong password | HTTP 401. |
| Secret-backed login / authenticated API | HTTP 200; cookies have Secure and correct Path. |
| Built frontend HTML/assets under prefix | Loaded; asset URLs retain prefix and return asset content. |
| Authenticated WebSocket | Upgrade succeeds; invalid/replayed ticket rejected. |
| Actual native Web chat | `session.create` resolves `qwen38-27b`; `prompt.submit` receives `WEB_SMOKE_OK`. Read-only SQLite confirms an assistant response was persisted. No tools requested. |
| Remove Telegram with old token retained in source Secret | Native enabled-platform list empty; rendered token empty. Native health can retain a `disconnected` Telegram entry; this is not an active consumer. Original Telegram spec restored. |
| Restart with stable Web credentials | Both keys unchanged; previously issued native session remains valid after Pod recreation. |
| Web-only → zero channels → Web-only | Route removed; native gateway remains running/Ready with no Telegram; keys preserved; prior login session works after re-enable. |
| Rotate both Web keys on temporary agent | Old password and old session rejected; new login succeeds. |
| Private egress while Web is enabled | Kubernetes Service `10.43.0.1:443` and node API `192.168.0.202:6443` refused; declared inference exception `192.168.0.210:443` reachable. |
| Primary user data | SOUL/personal files, skills, workspace, cron file hashes and conversation row/content hashes match pre-upgrade snapshot. |
| Delete temporary CR | Its Pod, Service, HTTPRoute and explicitly disposable PVC removed; source Secret retained without owner reference. |
| PVC identity | Primary `af337352-8dbe-4417-b015-a169b51cfc3d` unchanged. Older retained `runtime-smoke-data` untouched. |

The browser showed the native login page at the Path URL. Authenticated checks
used HTTP/WebSocket clients; this report does not claim a full manual tour of
every dashboard screen or a browser-driven PTY interaction. The native chat
RPC and persisted assistant result were exercised separately. No Telegram UI
messages were sent during this Web acceptance.

The aggregate unmanaged-config hash differs between the initial Telegram-only
snapshot and Web-enabled snapshot. The managed dashboard ownership set changes,
so this is not evidence of a byte-identical entire config. File/history hashes
above are independently equal; native managed configuration is restored from CR.

## Reproducibility and verification

- `make test-unit`: all internal Go packages pass.
- `make test-envtest`: CRD admission (including both complete Web examples),
  workload and controller pass against a real local API server without Gateway
  API installed; disabled Web remains functional.
- `make test-runtime`: 31 tests in the pinned official Hermes image, non-root,
  read-only root filesystem and dropped capabilities.
- `make test-e2e-harness`: 8 tests pass.
- Helm lint/render, docs links, generated CRD/RBAC/runtime checks and Go vet pass.
- Fresh independent branch review found one P2: upstream trims credentials.
  Password/signing-key regressions reproduced RED and pass after rejecting
  surrounding whitespace; username validation was aligned too.

[Web harness](../../hack/e2e-web.py) performs the read-only auth/transport checks.
[Lifecycle harness](../../hack/e2e-web-lifecycle.py) explicitly restarts, rotates
or toggles a named dedicated test instance. CLI examples and credentials
handling are in the [Web guide](../guides/web.md). Private temporary acceptance
files under `/tmp/hermes-web-acceptance` contain rollback configuration and
hash snapshots; credential values were never printed or committed.

## Release boundary

This is feature-branch acceptance, not a new release. Published Operator `0.1.0`
still lacks Web. Test dev images are separate amd64-only Docker Hub tags;
production release publishing remains the existing CI workflow for multiarch
GHCR images and Helm charts. Do not install the new CRD with an old controller
and expect Web fields to be acted upon. Gateway DNS/TLS and CNI behavior must
be verified again on another cluster.

Final smoke operator image: `docker.io/wbe7/hermes-operator:dev-web-1d260b1`
at `sha256:a4e8f88fc6fb7246ad933e48566094a4c7bbe780945d26bdc06cb6c82cebf13c`.
The development tag is deliberately separate from published release artifacts.
