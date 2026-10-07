# Operator 0.4.0 publication and Berger Apps upgrade — 2026-10-07

[PR #5](https://github.com/wbe7/hermes-operator/pull/5) was squash-merged as
`c011af8`. Both source workflows passed on that exact commit:
[CI](https://github.com/wbe7/hermes-operator/actions/runs/37632490760) and
[Hermes image](https://github.com/wbe7/hermes-operator/actions/runs/37632490858).
[Release CI](https://github.com/wbe7/hermes-operator/actions/runs/37633369164)
published the artifacts. This remains an experimental v1alpha1 release.

| Artifact | Reference |
| --- | --- |
| Operator | `ghcr.io/wbe7/hermes-operator:0.4.0` |
| amd64/arm64 index digest | `sha256:fe4e2a1101bbde458a540768cff323742f2739c70052618a5adee9d85187ba7f` |
| OCI chart | `oci://ghcr.io/wbe7/charts/hermes-operator:0.4.0` |
| Chart digest | `sha256:26c185617fdb26cdaf8191e53c8838cfdd447c8773daa21a18b760c6f6534cdb` |

Anonymous image manifest and chart retrieval passed. The image index contains
linux/amd64 and linux/arm64. All release attachment checksums passed; the OCI
chart archive matched the release attachment byte for byte. CI passed its
HIGH/CRITICAL fixed-vulnerability scan and published SBOM/provenance.
The bundled `compatibility.md` records pre-release evidence; this report and the
[current matrix](../reference/compatibility.md) add the actual 0.4.0 rollout.

## Upgrade and runtime verification

Upgraded Berger Apps (k3s v1.34.7+k3s1) from operator/chart 0.3.0 to 0.4.0,
Helm revision 7 in `hermes-operator-system`. Applied the published CRD before
Helm using a resourceVersion-guarded JSON patch of `spec.versions`, retaining
existing shared ownership without forcing it or deleting the CRD. Retained
existing Helm values and pinned the published image index digest. Helm used
`--atomic --wait`; the actual Deployment image matched the selected digest.

- Controller Ready; inspected startup logs contained no error-level entries.
- All 16 active installations passed native Hermes configuration checks for the
  new TTS/voice ownership and existing STT/model settings. One previously
  suspended installation stayed suspended.
- All 16 external web roots returned HTTP 200 over verified TLS after rollout.
  This availability check does not repeat the full authorization/WebSocket suite.
- Agent image/version remains the pinned Hermes v2026.9.14 catalog entry.
  Agent Pods rolled to apply the new managed TTS defaults.

## TTS and STT smoke

`hermes-smoke` has `spec.tts.enabled: true`, `model: fish-s2-pro`, `voice: default`
and `responseMode: VoiceOnly`. URL and credentials inherit its existing LLM
configuration. STT stays enabled with `qwen3-asr-1.7b`, Russian default and
transcript echo. No credentials were rotated; LLM reasoning remains xhigh.
Other installations have no TTS opt-in and explicitly disable it.

Native Hermes generated voice-compatible Ogg/Opus at 48 kHz using the actual
operator-managed home/configuration. Native STT transcribed that file as
«Это проверка синтеза речи. Номер теста 42.» The same chain passed after an
explicit same-Pod SIGTERM/restart (restart count 0 → 1). Generated audio lasted
4.046792 seconds before and 3.907458 seconds after restart.

The restart harness verified restored managed settings, unchanged personal
configuration/files and unchanged SQLite history. The final cluster-wide audit
found no personal file or history digest differences across all 16 active agents.
All agent-namespace PVC UIDs and bound volume names were preserved. CR specs
remained unchanged except the requested smoke TTS section; resolved agent images
were unchanged.

## Telegram delivery boundary

A separate outbound test verified the bot identity as `@HermesOperatorK8SBot`,
generated Opus with native Hermes TTS, and attempted `sendVoice` only to the
already-authorized smoke user's personal chat. Telegram rejected it with
`VOICE_MESSAGES_FORBIDDEN`. No second `getUpdates` consumer was started and no
other chat was used. This is a delivery restriction for the receiving chat, not
a speech synthesis or codec failure. Recipient voice-message permissions must
allow the bot before voice-bubble delivery can be verified.

The macOS Telegram UI was unavailable because the Mac was locked. Therefore this
rollout does **not** claim a complete incoming voice → LLM → outgoing voice test.
Earlier chat history also showed LLM timeouts; an independent short public LLM
request returned HTTP 200 and its first stream line in 6.27 seconds, which does
not prove the full Hermes context path. The documented native configuration,
TTS/STT, restart and preservation checks passed independently of these limits.

See [TTS configuration](../guides/tts.md) and the
[backend Opus fix evidence](tts-2026-10-06.md). Private rollback inputs and test
evidence are retained in `/tmp/hermes-release-0.4.0/`; they are not release
attachments. This rollout does not close unrelated
[qualification gaps](v1-acceptance.md), including unauthorized Telegram identities
and data-schema migration/rollback.
