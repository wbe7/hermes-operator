# Runtime verification — 2026-09-18

Official image: `nousresearch/hermes-agent@sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294`, release `v2026.9.14`, source revision `345cd2b057a452236de401d3534b8502a7465e8d`. No upstream source, installed package or image was patched.

## Environment and reproducible checks

Docker client/server `29.5.3`; daemon `linux/arm64`; image Python `3.13`. Official digest was pulled and verified by the coordinating task. Local Docker credential helper hung on the initial pull; an empty Docker config for the public image fixed that prerequisite. Commands in this session used:

```sh
export DOCKER_CONFIG=/tmp/hermes-docker-public
export DOCKER_HOST=unix:///Users/mtik/.docker/run/docker.sock
docker version --format '{{.Client.Version}} {{.Server.Version}} {{.Server.Os}}/{{.Server.Arch}}'
test/runtime/run.sh
```

Test command runs the pinned image with UID/GID 10000, read-only root filesystem, all capabilities dropped, `no-new-privileges`, `/tmp` tmpfs, and runtime scripts mounted read-only. `PYTHONPATH=/runtime:/opt/hermes` belongs only to the test process. Production bootstrap and probes use `python -I`, explicitly import only the mounted runtime and `/opt/hermes`, and never put writable workspace on `sys.path`.

Initial preservation test failed locally with `ModuleNotFoundError: bootstrap` before implementation. A replay of that unchanged test-only mount in the official image also exited 1 with that error. The later official-image suite passes 15 tests, including 40 synthetic Telegram handler cases. Upstream native SessionStore tests emit `ResourceWarning` about cached unclosed SQLite handles at test process exit; these warnings are recorded, not suppressed.

Covered behavior:

- Tuple-path merge, list leaves, null removal, retired keys, idempotence, ownership collisions, atomic same-directory rename and fsync.
- Pending plus committed ownership recovery; errors preserve data and reveal no credential values.
- Native SessionStore session creation, `/model`-shaped persisted override, transcript append, unchanged session ID/history after reset, primary SQLite and JSON mirror. The mirror's `_README` string is retained. Corrupt SQLite fails closed without clearing the file.
- Selected custom provider pool and matching legacy custom-provider credential reset, preservation of unrelated credentials, and the actual upstream effective provider resolver afterward. YAML comparison alone is not the assertion.
- Selective per-channel model/provider reset retains personal channel prompts.
- Readiness checks current process identity, Telegram writer identity, session store and gateway state. Liveness checks local heartbeat and the UNIX loop-tick socket and ignores external provider/Telegram outage state.

## Real restricted container startup and restart

Two `--restore-only` runs on the same `/opt/data` bind mount exited 0. The production launch shape was:

```sh
docker run --pull=never --user 10000:10000 --read-only \
  --cap-drop=ALL --security-opt=no-new-privileges --tmpfs /tmp:rw,nosuid,nodev \
  --entrypoint /opt/hermes/.venv/bin/python \
  -v "$PWD/runtime:/operator/runtime:ro" \
  -v /tmp/hermes-runtime-live/config:/operator/config:ro \
  -v /tmp/hermes-runtime-live/credentials:/operator/credentials:ro \
  -v /tmp/hermes-runtime-live/home:/opt/data \
  nousresearch/hermes-agent@sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294 \
  -I /operator/runtime/bootstrap.py
```

Credentials supplied later by the user were read from ignored mode-600 `.env` into temporary credential files. No values appear in evidence or repository fixtures. One temporary live Telegram poller was started; restart notifications were disabled and no messages were sent. `probe.py ready` exited 0 with live Telegram connected, session store OK, current process and functioning loop tick. The actual upstream resolver confirmed declared model/provider/base URL/key and `xhigh` without sending a provider request.

The container was stopped, local model/reasoning changed, personal prompt/workspace/profile fixtures added, then the **same container** was started. Bootstrap restored declared model and `xhigh`; SOUL, personal prompt, workspace file and saved profile remained. The temporary poller was stopped after verification, before the coordinating task could start a cluster poller.

This proves local official-image startup, real Telegram connection, effective configuration and same-container restoration. It does **not** prove a real user DM/model response, conversational onboarding, or same-Pod Kubernetes restart; those remain end-to-end acceptance work.

## Accepted Telegram limitation

Real upstream text, command, media (sticker with fake download/vision transport), and callback handler entrypoints were invoked with synthetic identities. Sender authorization uses the upstream `GatewayAuthorizationMixin`. Text/command/media enforce sender AND chat allowlists, and strangers are rejected in all tested paths.

A pending inline picker callback (`cp:0`) from an otherwise allowed sender bypasses `allowed_chats`: the real picker invokes its `on_choice_selected` callback in a forbidden group. That callback can change reasoning; it is not merely an expired-button no-op. The initial matrix had three failing subcases (`1 != 0`) for these allowed-sender/forbidden-group combinations. The user explicitly accepted this limitation for v1; the regression matrix now asserts that exact upstream behavior while still rejecting unauthorized senders. No monkey patch was added. `v2026.9.14` was still the latest release when checked through GitHub releases API.

## Version-specific health facts

`gateway_state.json.start_time` is a Linux `/proc` start-tick fingerprint. Heartbeat `start_time` is the runner's epoch start time, **not the same unit**. The adapter uses upstream `get_runtime_status_running_pid(..., expected_home=home)` for identity, checks heartbeat PID and that its epoch falls after live process creation, checks monotonic freshness, and probes the current PID's UNIX socket. The strict lock-only identity helper returned no PID during actual direct `--no-supervise` startup; the upstream runtime-status identity helper correctly verified the process. Idle JSON snapshot age is not used as a liveness deadline.

## Remaining boundary

Runtime adapter support is pinned to this release. `restore` rejects an unknown release/schema. This is a runtime verification result, not a claim that the operator's Kubernetes, network isolation, upgrade, Secret rotation or end-to-end conversation acceptance gates have passed.
