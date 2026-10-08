# Operator 0.4.1 publication and Berger Apps upgrade — 2026-10-08

[PR #7](https://github.com/wbe7/hermes-operator/pull/7) was squash-merged as
`dcd45ec`. Both source workflows passed on that exact commit:
[CI](https://github.com/wbe7/hermes-operator/actions/runs/37779061826) and
[Hermes image](https://github.com/wbe7/hermes-operator/actions/runs/37779061840).
[Release CI](https://github.com/wbe7/hermes-operator/actions/runs/37779827584)
published the artifacts. This remains an experimental v1alpha1 release.

## Permanent automatic TTS fix

The native automatic voice-reply path previously chose `/tmp/hermes_voice`,
which was outside Hermes' write-safe root. Operator 0.4.1 sets
`TMPDIR=/opt/data/.cache/tmp` and mounts the existing bounded 1 GiB `tmp` emptyDir
there as well as at `/tmp`. Temporary audio remains ephemeral; the fix does not
consume the personal PVC or weaken file protection. The Hermes core, agent image,
privileges, model, reasoning and credentials remain unchanged.

The pinned-image regression executes the actual native automatic synthesis
function with safe-root enforcement and session-platform context cleared, matching
the post-handler gateway path. It checks the real speech SDK's Opus request and
returned audio. See the [incident analysis](auto-tts-temp-2026-10-07.md).

## Published artifacts

| Artifact | Reference |
| --- | --- |
| Operator | `ghcr.io/wbe7/hermes-operator:0.4.1` |
| amd64/arm64 index digest | `sha256:a4678eb05379c43b14ad2bdac9d960a8a931ea31484f8eb547d2e3b14b3069bc` |
| OCI chart | `oci://ghcr.io/wbe7/charts/hermes-operator:0.4.1` |
| Chart digest | `sha256:684874a07f054809c349d9888c9f2c33129e3ede4a37189a93cc0c619b573485` |

Anonymous image manifest and chart retrieval passed. The index includes
linux/amd64 and linux/arm64. All release attachment checksums passed; the OCI
chart archive matched the release attachment byte for byte. Release CI passed
its fixed HIGH/CRITICAL vulnerability scan and published SBOM/provenance.
The bundled compatibility document contains pre-release evidence; this report
and the [current matrix](../reference/compatibility.md) add live rollout results.

## Upgrade and preservation

Upgraded Berger Apps from operator/chart 0.4.0 to 0.4.1, Helm revision 8 in
`hermes-operator-system`. The published CRD schema was already installed and
needed no mutation. Existing Helm values were retained, the new image was pinned
by its published index digest, and Helm used `--atomic --wait`.
The live Deployment image matched the published digest; the controller had one
Ready replica and its inspected startup logs had no error-level entries.

- All 16 active installations passed native configuration and safe temporary-path
  checks. One previously suspended installation remained suspended.
- For every active agent, the native temporary-file API used
  `/opt/data/.cache/tmp`; an actual write there was visible through the same
  bounded volume at `/tmp`. Native file protection accepted the new path and
  still denied `/tmp/hermes_voice` as outside the safe root.
- Removed `hermes-smoke.spec.extraEnv.TMPDIR` and verified the standard environment
  took effect. The obsolete diagnostic directories were inspected and removed
  only once confirmed empty. No temporary configuration workaround remains.
- The smoke same-Pod SIGTERM test passed: restart count 0 → 1, restored managed
  configuration, unchanged personal configuration/files and SQLite history.
- All agent-namespace PVC UIDs and bound volume names were preserved, as were
  agent image pins and CR specs except for removal of that smoke override.
  Personal file hashes matched. One active agent gained new conversation history;
  a read-only comparison proved its entire pre-upgrade history prefix unchanged.
- All 16 external web roots returned HTTP 200 over verified TLS. This is an
  availability check, not a rerun of the full authorization/WebSocket suite.

The cluster-wide snapshot initially had one transient Kubernetes exec connection
reset; repeating only the interrupted read succeeded. No affected Pod restart or
repair was needed.

## Native automatic voice smoke

The actual pinned Hermes `_synthesize_auto_tts()` ran with the managed smoke
configuration and without process-local overrides. Native VoiceOnly dispatch
checks selected voice input and rejected text input. Opus output was 48 kHz,
3.675271 seconds before the controlled restart and 3.582375 seconds after it.
Native STT recognized both as «Проверка голосового ответа. Номер теста 42.».

The smoke still uses `fish-s2-pro`, voice `default`, `VoiceOnly`, and
`qwen3-asr-1.7b` with Russian STT. URL/key inherit the LLM settings. The LLM model
remains `qwen38-27b` with xhigh reasoning. No credentials or agent image changed.

## Telegram scope

A separate outbound check verified the bot identity and sent the native automatic
function's Opus output only to the authorized smoke user's personal chat. The
Bot API client timed out while waiting for `sendVoice`, but the exact test voice
bubble and its caption were then observed in `@HermesOperatorK8SBot` at 16:21 MSK.
It was not retried, avoiding a duplicate after an ambiguous transport outcome.
The earlier recipient `VOICE_MESSAGES_FORBIDDEN` restriction no longer blocked
this delivery.

The desktop client could display the authorized chat, but click actions failed
with `noWindowsAvailable`. The user therefore sent a fresh two-second voice
message at 16:24 MSK and explicitly confirmed doing so. The complete live flow
then passed:

- Native Telegram cached that incoming voice at 13:24:37 UTC and STT transcribed
  it at 13:24:39 UTC using `qwen3-asr-1.7b`; the transcript echo was visible in chat.
- Read-only native history correlated the incoming platform message with a new
  user/assistant turn for the permitted Telegram identity. Accounting increased
  by one `qwen38-27b` API call, 14,522 input tokens and 298 output tokens; no tool
  call was recorded for this turn.
- The gateway automatically invoked TTS at 13:25:05 UTC and saved the 47,440-byte
  output under `/opt/data/.cache/tmp/hermes_voice` at 13:25:30 UTC.
- A six-second voice reply and its matching text were observed in
  `@HermesOperatorK8SBot` at 16:25 MSK. No manual speech command, `/voice` override,
  extra restart or second Bot API consumer was used for this exchange.

This verifies incoming Telegram voice → STT → LLM → automatic TTS → voice delivery
on the deployed 0.4.1 smoke after its controlled restart. It is a single authorized
personal-chat test, not multi-account/group acceptance or a latency benchmark.

Private rollback inputs and verification evidence are retained in
`/tmp/hermes-release-0.4.1/`; they are not release attachments. This release does
not close unrelated [qualification gaps](v1-acceptance.md).
