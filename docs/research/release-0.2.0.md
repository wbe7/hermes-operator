# Operator 0.2.0 publication and Berger Apps upgrade — 2026-09-28

[PR #3](https://github.com/wbe7/hermes-operator/pull/3) was squash-merged as
`a8fd8f7`. Release source `64ce4f3` also aligns the chart version/appVersion.
[Source CI](https://github.com/wbe7/hermes-operator/actions/runs/36403954829)
and [release CI](https://github.com/wbe7/hermes-operator/actions/runs/36404668328)
passed. This remains an experimental v1alpha1 release.

| Artifact | Reference |
| --- | --- |
| Operator | `ghcr.io/wbe7/hermes-operator:0.2.0` |
| amd64/arm64 index digest | `sha256:40db64b1c40479917645ed576cfca0b988a80acb2aaf19405b3ee6f3b3012baa` |
| OCI chart | `oci://ghcr.io/wbe7/charts/hermes-operator:0.2.0` |
| Chart digest | `sha256:9577b2f7b09cbbdc8a6fc48eef1b6b90214d03cc2c367e19e9e59ef4f5bc4adb` |

Anonymous manifest/chart retrieval succeeded. The image index contains linux/amd64
and linux/arm64 plus attestation manifests. Release attachment checksums passed;
the OCI chart archive matched the release attachment byte for byte. Release CI
also passed its HIGH/CRITICAL fixed-vulnerability scan.

## Upgrade and verification

Applied the published CRD, then upgraded `hermes-operator` in
`hermes-operator-system`, context `berger-apps`, to chart 0.2.0, revision 5.
Retained cluster network/resource values; replaced the dev image override with
the published GHCR image and index digest above. Helm used `--atomic --wait`.
The prior image was `docker.io/wbe7/hermes-operator:dev-web-1d260b1`;
this was a dev-Web-to-release upgrade, not a direct 0.1.0-to-0.2.0 acceptance.

- Deployment rollout succeeded; running image matches the selected digest.
- New controller acquired and renewed the leader Lease; inspected startup logs
  contained no error-level entries.
- All 10 Hermes installations retained their specs/resolved images and Ready=True.
- All 10 agent Pods retained their UID and restart counters; no agent restart.
- All 11 PVCs in the agent namespaces retained UID and bound volume identity.
- Smoke native loader, model/provider/reasoning, personal configuration digest,
  personal file hashes and session/history digests were unchanged.
- All primary smoke credential hashes were unchanged.
- External smoke URL passed TLS, native auth, anonymous/wrong-password rejection,
  authenticated API, frontend assets, Secure/path cookies, WebSocket upgrade and
  single-use ticket replay rejection.

The agent image remains pinned to the existing Hermes v2026.9.14 catalog entry.
No Telegram messages were sent and no new conversational model turn was performed
during this upgrade. Hermes Desktop connectivity was not tested. The native Web
chat embeds a TUI; this release adds no separate graphical chat frontend.

Private local evidence and rollback inputs: `/tmp/hermes-release-0.2.0/`.
These are not release attachments. The earlier Web lifecycle tests remain in
[Web acceptance](web-acceptance-2026-09-28.md); this upgrade does not replace them
or claim complete production qualification.
