# Optional Web Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide opt-in native Hermes Web through Gateway API, independently of Telegram, with durable credentials in the primary Secret.

**Architecture:** Keep the existing persistent home, revision snapshots and non-root container. Add typed web configuration, pure route/network builders, controller-managed credentials and publication, and a launcher supervising native gateway/dashboard. Gateway API resources are accessed dynamically so clusters without its CRDs retain Telegram/zero-channel operation.

**Tech Stack:** Go/controller-runtime, Kubernetes Gateway API v1 via unstructured objects, Python runtime adapter, Helm, unchanged Hermes v2026.9.14.

**Spec:** [Approved Web contract](../../design/web-access.md).

## Global Constraints

- Web defaults off; Telegram, Web, both and neither are valid. Existing Telegram CRs keep their meaning.
- Native dashboard/password auth only; no upstream patches, additional auth proxy, DNS or certificate provisioning.
- `WEB_PASSWORD` and `WEB_SESSION_SECRET` live in the primary Secret; generated only when absent, stable across restarts/disable/delete.
- Preserve PVC/state, rootless isolation and private-egress policy. Never inspect Secret values or personal state in logs.
- External smoke uses `agents.s2technologies.ru`, never substitutes `ext`. DNS and nested wildcard TLS must be verified before exposure.
- Path shares browser origin; docs recommend Subdomain for mutually untrusted users.
- Execute in existing feature branch `codex/web-access`; user explicitly approved implementation. Do not repeat the design/start approval.

## Review Focus

- Removing Telegram while old credentials/config remain must not silently enable it (Task 1/3 tests).
- Missing Gateway CRDs and stale parent status must not crash the operator or falsely declare a URL ready (Task 4 tests).
- Credential key deletion, immutable Secret and concurrent Secret updates must not overwrite unrelated data or leak bytes (Task 4 tests).
- Disabling/suspending/changing invalid Web configuration must remove old publication without deleting user state (Task 4 tests).
- Prefix spoofing, wrong HTTPS metadata and dashboard process death must not expose an unguarded interface or give false readiness (Task 2/3/5 tests).

### Task 1: Optional channels and typed Web configuration

**Files:** `api/v1alpha1/hermes_types.go`, new `api/v1alpha1/web_types.go`, `internal/config/{render,secrets,validate}.go`, new `internal/config/web.go`, relevant tests and generated CRD/deepcopy/chart copies.
**Interfaces:** Retain existing `TelegramSpec` value type for Go callers, add explicit JSON omission detection only if needed; prefer pointer and update existing test fixtures consistently. Add `WebSpec`, `WebRoutingSpec`, `WebGatewayRef`, `WebAuthSpec`, `WebNetworkSpec`. Pure helpers `WebEnabled(h)`, `WebAddress(h) (host, prefix, url string)`, `PrimarySecretName(h) string` in config. Startup bundle adds immutable `channels` with telegram/web booleans.
- [ ] Write failing tests for optional Telegram, both route addresses/default name, invalid host/mode/selector/proxy, managed dashboard config conflict and credential references.
- [ ] Run focused config tests and observe missing behavior.
- [ ] Implement typed schema, validation, conditional Telegram render/bindings and reserved dashboard config/env. Always clear disabled Telegram token/enable aliases. Render native auth/public URL/trusted proxies from managed config.
- [ ] Run all unit tests; regenerate and validate CRD/deepcopy/chart. Expected: tests pass, generated assets match.
- [ ] Commit API/config changes.

### Task 2: Web resources and ingress

**Files:** new `internal/web/{resources,resources_test}.go`, `internal/network/{policy,policy_test}.go`, `internal/workload/{build,build_test}.go`.
**Interfaces:** `web.Build(h) (*corev1.Service, *unstructured.Unstructured)` builds owned `<name>-hermes-web` resources, service port 9119. Ingress peers are typed bounded `web.network.ingressFrom`; proxy CIDRs explicit and bounded. `web.RouteReady(route, h) bool` compares selected parent, current generation, Accepted/ResolvedRefs.
- [ ] Tests assert exact hostname/path/filter/backend, PrefixRewrite+SET prefix, default deny when off, bounded peers/port, source immutability, stale statuses rejected.
- [ ] Run tests RED then implement builders using Gateway API v1 and preserve existing headless Service identity.
- [ ] Run network/web/workload suite GREEN; commit.

### Task 3: Native dashboard lifecycle and probes

**Files:** `runtime/bootstrap.py`, new `runtime/supervisor.py`, `runtime/probe.py`, `runtime/adapters/v20260914.py`, tests under `test/runtime/`, regenerated embedded assets.
**Interfaces:** Startup input `channels` consumed from immutable bundle; default legacy bundle means Telegram-only. Supervisor starts native gateway and optionally dashboard with fixed flags/isolated Python, forwards signals/reaps children and exits on essential child death. Probes require native gateway identity/health and enabled channels only; dashboard probe checks guarded native status, expected provider and a live process identity.
- [ ] Add failing tests for zero-channel readiness, stale Telegram configuration, supervisor termination/child failure, missing dashboard/auth readiness.
- [ ] Run RED; implement launcher and probes without constructing SessionStore against a live home.
- [ ] Run runtime suite GREEN, regenerate assets and run Go workload/config tests; commit.

### Task 4: Credentials and publication reconciliation

**Files:** new `internal/controller/{web,web_credentials,web_test,web_credentials_test}.go`, `hermes_controller.go`, `status.go`, `dependencies.go`, RBAC and Helm generated copies.
**Interfaces:** `ensureWebCredentials(ctx,h) error`, `removeWeb(ctx,h) error`, `reconcileWeb(ctx,h) error`; use direct API reader/dynamic unstructured operations, no mandatory Gateway informer. Requeue covers route and Gateway changes. Web condition/status carries URL and object identity, never values.
- [ ] Tests cover generated/provided/missing/empty/immutable credentials, optimistic conflict, preservation and stable revision.
- [ ] Tests cover absent CRDs, listener HTTPS/allowed attachment, owned resource conflicts, managed address collision, current parent conditions, disable/suspend/delete and invalid desired configuration cleanup.
- [ ] Implement credential provisioning before dependency resolution; validate before credential mutation. Preserve original Secret ownership and unrelated keys. Never auto-delete source credentials.
- [ ] Implement guarded web preflight/apply/cleanup, sanitized errors and status. Add minimal Gateway read/HTTPRoute manage RBAC. Existing workloads must remain isolated on failure.
- [ ] Run full unit/envtest/runtime/Helm/generated checks GREEN; commit.

### Task 5: Live acceptance, docs and branch review

**Files:** new web examples and `docs/guides/web.md`, README/API/install/operations/security updates, `hack/e2e-web.py` and tests, dated live report.
**Interfaces:** Use existing kubeconfig/context and smoke CR with its PVC. Do not print tokens. A temporary Web-only instance has independent credentials/storage and no Telegram consumer.
- [ ] Add executable browser/HTTP acceptance covering login/auth rejection, WS chat, both routes, two web instances, persistence, credential rotation and disable/re-enable. Run harness self-tests.
- [ ] Verify image supports native dashboard/TUI and deployment preconditions; build/test operator and deploy authorized smoke upgrade with rollback reference. Never change DNS/TLS implicitly; report if user infrastructure change is pending and continue independent tests.
- [ ] Exercise four channel combinations and external URLs where DNS/TLS permit, preserve original smoke data, record exact evidence and untested cases.
- [ ] Update guides/examples with complete CRs, Secret extraction/rotation, explicit Gateway ingress/proxy settings, cert/wildcard needs, optional CRDs and accepted auth/path boundaries.
- [ ] Run full checks and one fresh whole-branch reviewer under executing-plans. Fix important findings with regression tests, then commit docs/evidence and report result without claiming unexecuted tests.
