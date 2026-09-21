# Operator 0.1.0 publication and Helm — 2026-09-21

Published experimental [release](https://github.com/wbe7/hermes-operator/releases/tag/operator-v0.1.0),
source `5994ef7fba894223b464a5cb6896befb74a6b21c`. This is an artifact and deployment
checkpoint, not a claim that every v1 acceptance case passed.

| Artifact | Reference |
| --- | --- |
| Operator | `ghcr.io/wbe7/hermes-operator:0.1.0` |
| Operator amd64/arm64 index | `sha256:6725c42f2bc0717dfdfc289fbeafae22402636d44bb4a4d5a12169444935991b` |
| Helm chart | `oci://ghcr.io/wbe7/charts/hermes-operator`, version `0.1.0` |
| Chart OCI digest | `sha256:168c9f467470a72f07b0c6a0ad746c12a71aca97170c039401e31f8d51d03664` |

[Release CI](https://github.com/wbe7/hermes-operator/actions/runs/35627925966)
passed source/chart/generated/docs checks, the image scan and multiarch publication
with SBOM/provenance. GitHub Release includes the chart archive, CRD, compatibility
matrix and SHA256 checksums. Anonymous image manifest and Helm pulls succeeded;
the OCI chart archive matched the checksummed release attachment byte for byte.
Checksums verify content integrity; they are not a detached artifact signature.

## Berger Apps migration

Release `hermes-operator`, namespace `hermes-operator-system`:

1. Saved the previous Deployment, RBAC, network settings and read-only agent state.
2. Confirmed the released CRD spec matched the installed CRD and RBAC rules were
   unchanged. No CRD schema change was required for this migration.
3. Added Helm ownership metadata to the existing operator ServiceAccount, RBAC
   and settings. Recreated only the operator Deployment because its dev selector
   differed from the chart's immutable selector.
4. Installed the published chart (Helm revision 1), then performed a same-version
   upgrade to pin the operator image digest (revision 2).
5. Verified the running image digest, Ready controller, renewing leader Lease and
   absence of error-level startup logs. In-Pod source compilation was removed.

The native agent retained its Pod UID and had zero container restarts across
both operations. Thirty health samples were ready. The CR/spec/applied revision,
PVC/PV identity, NetworkPolicy, 348 checked personal files, 384 historical messages
and personal configuration remained unchanged. Native runtime checks confirmed
`qwen38-27b`, `xhigh`, the declared credential and removal of the old managed
OpenRouter alias without printing credentials. SQLite assertions were read-only;
no external SessionStore was instantiated against the live home.

This validates Helm ownership migration and a same-version configuration upgrade
with a Ready gateway. The Helm lifecycle subcheck passed; the cross-version
operator upgrade subcheck remains `not-run`. Consequently, aggregate A12 remains
`not-run (partial passed)` in the [acceptance ledger](v1-acceptance.md). A future
operator-version or data-schema upgrade needs its own acceptance; no automatic
downgrade guarantee is made.

Local private evidence is under `/tmp/hermes-release-0.1.0/`, including
`install-verification.json`, `upgrade-verification.json`,
`operator-verification.json` and `upgrade-health-samples.json`. Snapshots and
rollback manifests are not release attachments. The subsequent intentional reset
of this smoke installation is described in [fresh smoke](fresh-smoke-2026-09-21.md).
