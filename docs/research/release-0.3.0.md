# Operator 0.3.0 publication and Berger Apps upgrade — 2026-10-05

[PR #4](https://github.com/wbe7/hermes-operator/pull/4) was squash-merged as
`12a3aab`. Both source workflows passed on that exact commit:
[CI](https://github.com/wbe7/hermes-operator/actions/runs/37292376753) and
[Hermes image](https://github.com/wbe7/hermes-operator/actions/runs/37292376832).
[Release CI](https://github.com/wbe7/hermes-operator/actions/runs/37293036984)
published the artifacts. This remains an experimental v1alpha1 release.

| Artifact | Reference |
| --- | --- |
| Operator | `ghcr.io/wbe7/hermes-operator:0.3.0` |
| amd64/arm64 index digest | `sha256:4d4d082c44d6ca23b120ad3707de42b6717dba68992f06faf6ab106773f186a4` |
| OCI chart | `oci://ghcr.io/wbe7/charts/hermes-operator:0.3.0` |
| Chart digest | `sha256:d383ea5177133c638edae098cd2aaca67f3b11a3facac5db4f6b337e85e7e6f2` |

Anonymous image manifest and chart retrieval passed. The image index contains
linux/amd64 and linux/arm64. All release attachment checksums passed; the OCI
chart archive matched the release attachment byte for byte. CI passed its
HIGH/CRITICAL fixed-vulnerability scan and published SBOM/provenance.

## Upgrade and runtime verification

Upgraded Berger Apps (k3s v1.34.7+k3s1) from operator/chart 0.2.0 to 0.3.0,
Helm revision 6 in `hermes-operator-system`. Applied the published CRD before
Helm. Existing shared ownership of `spec.versions` prevented ordinary SSA in
preflight; a resourceVersion-guarded JSON patch installed the exact release
schema without forcing ownership or deleting the CRD. Retained existing network
and resource values and pinned the published index digest. Helm used
`--atomic --wait`; the actual Deployment image matched the selected digest.

- Controller Ready; inspected startup logs contained no error-level entries.
- All 16 active installations returned to Ready and passed native Hermes
  configuration checks. One previously suspended installation stayed suspended.
- All 16 external web roots returned HTTP 200 over verified TLS before and
  after rollout. This availability check does not repeat the complete Web
  authorization/WebSocket acceptance suite.
- Final audit confirmed identical personal file hashes and conversation/history
  digests for all 16 running agents. All agent-namespace PVC UIDs and bound volume
  names were preserved. CR specs remained unchanged except the requested smoke
  STT configuration.
- Agent image/version remains unchanged at the pinned Hermes v2026.9.14 catalog
  entry. Agent Pods rolled to load new managed STT defaults.

## STT

`hermes-smoke` now has `spec.stt.enabled: true` and
`spec.stt.model: qwen3-asr-1.7b`. Russian language and echo defaults apply;
endpoint and key inherit from its existing LLM configuration. No key was rotated.
Other installations have no STT opt-in and therefore explicitly disable it.

Native Hermes transcription inside the smoke Pod returned the exact synthetic
Russian phrase: «Это проверка распознавания речи. Номер теста сорок два.»
After an explicit same-Pod SIGTERM/restart, native transcription returned the
same phrase again. Personal configuration, file and history hashes were unchanged.

Real Telegram voice transport was not exercised in this rollout; no Telegram
messages were sent. Detailed native fixtures and the earlier development smoke
are recorded in [STT acceptance](stt-2026-10-05.md).

Private rollback inputs and evidence: `/tmp/hermes-release-0.3.0/`.
They are not release attachments. Successful rollout does not close unrelated
[qualification gaps](v1-acceptance.md), including unauthorized Telegram identities
or data-schema migration/rollback.
