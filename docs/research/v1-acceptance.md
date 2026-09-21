# v1 acceptance ledger

Latest release/deployment checkpoint: [operator 0.1.0 and Helm](release-0.1.0.md).
Latest clean-home validation: [fresh smoke](fresh-smoke-2026-09-21.md).
Earlier source review remains in [final validation](final-validation.md).

Status: **experimental 0.1.0 published; full qualification incomplete**. A row passes only when all its required subchecks
pass. The executable local suite deliberately does not turn synthetic fixture
success into native Hermes or Telegram acceptance.

| ID | Status | Executed evidence / remaining gate |
| --- | --- | --- |
| A01 | not-run (partial passed) | Fresh Berger A→B→A run verified separate namespaces/CRs/PVCs, effective xhigh versus medium, no cross-installation marker, and persistence on return to A. Both became natively Ready sequentially with one shared test token; two simultaneously Ready installations with independent credentials remain unverified. [Fresh evidence](fresh-smoke-2026-09-21.md). |
| A02 | passed | Fresh home had zero native messages/sessions. Authorized Telegram DM produced a visible nonce reply, matching native user/chat identity and new qwen38-27b token accounting. SOUL and persistent USER memory contained the onboarding nonce and preferences. [Fresh evidence](fresh-smoke-2026-09-21.md). |
| A03 | not-run | Offline upstream filter tests exist (`make test-runtime`); actual unauthorized sender/group text/commands/media and accepted old-inline-callback limitation require dedicated Telegram identities. |
| A04 | passed | Fresh Telegram `/reasoning low --global` was natively confirmed, then same-Pod SIGTERM restored CR xhigh/model/provider/credential. All 340 personal file hashes, 20 real Telegram history messages and personal config survived; restart count 0→1. [Fresh evidence](fresh-smoke-2026-09-21.md); [earlier checks](berger-apps-preflight.md). |
| A05 | passed | Fresh Pod deletion/recreation changed Pod UID and retained PVC, 340 personal file hashes, the native session and 20 real Telegram history messages. Post-restart Telegram reply recalled names, workspace marker and skill, with native model accounting. [Fresh evidence](fresh-smoke-2026-09-21.md); earlier controller-driven rollout evidence remains in [Berger preflight](berger-apps-preflight.md). |
| A06 | passed | Real controller extraConfig add/remove produced new immutable inputs; original-image restore-only on the same actual PVC removed the previously managed compression threshold while preserving adjacent personal config and SOUL. `/tmp/hermes-operator-deploy/extra-removal.json`; no Telegram consumer needed for this bootstrap check. |
| A07 | passed | Actual Berger created Retain/Delete/existing claims, marker preserved across expansion, no automatic adoption; `/tmp/hermes-operator-deploy/storage-lifecycle.json`. Local repeat also checks persistence after Helm uninstall. |
| A08 | passed | Deleted/missing Secret and unsupported version confirmed by `controller-revocation.json`; offline corruption tests exist. Berger disposable original-image fixture proved corrupt YAML/SQLite and PVC permissions fail closed without reset and recover after repair using restore-only; `corrupt-home.json`. This does not claim fake-token Telegram Ready. Actual Telegram token conflict remains not-run (no second real-token consumer). |
| A09 | not-run (partial passed) | Original-image UID10000/CapEff0/NoNewPrivs/no SA mount; generated policy on actual Berger Pod; reference Calico native dual-stack DNS/private/Service/node/exact exceptions. [Local evidence](local-dual-stack.md). External publicIPv6 unavailable in unrestricted baseline; controlled local metadata-address stand-in added in review fix; execution result recorded below. |
| A10 | passed | Controller suspend/revocation/resume on preserved PVC; manager SIGTERM preserved gateway Pod UID and revision (`manager-restart.json`). Telegram cold-start outage recovered without restart storm; `telegram-outage-results.json`. Actual provider IP exception removed for 100 seconds: six idle health samples remained live/ready with zero restarts and same Pod/revision; rule restored and TCP recovered (`provider-outage.json`). No active user turn was simulated. |
| A11 | passed | Berger CSI WaitForFirstConsumer + actual 1Gi→2Gi expansion retained marker; shrink admission rejection in schema tests. `storage-lifecycle.json`. Local kind storage class does not advertise expansion; no local expansion claim. |
| A12 | not-run (partial passed) | **Passed — Helm lifecycle:** local install/explicit CRD update/upgrade/uninstall and retained PVC checks with an active Pending CR; Berger published 0.1.0 install and same-version digest-pinning upgrade with a Ready native gateway, unchanged Pod, 30 health samples and matching personal/history hashes. [Evidence](release-0.1.0.md). **Not-run — cross-version operator upgrade:** an active CR surviving a change of operator version remains unverified, so the full Task 8 upgrade gate is open. Data-schema upgrades are also unverified; no automatic data rollback claim. |

Raw Berger artifacts are local sanitized run records, not downloadable release
attachments. Committed narrative evidence is [Berger preflight](berger-apps-preflight.md).
Network raw records live under `/tmp/hermes-e2e-dual`; final harness evidence uses
`E2E_DIR` with environment/image identity, JSON checks and `tests.txt`. Do not
publish kubeconfigs, Secret manifests, credential files, raw gateway logs, user
conversation content or live-state snapshots.

## Reproduce

- `make test-e2e`: disposable native dual-stack kind/Calico + actual Helm install,
  generated-policy probes, controller and PVC lifecycle. Cleans up on failure.
- `make test-runtime`: original-image native API/restore/filter tests, offline
  transport. This is not a real Telegram conversation.
- `make test-e2e-telegram`: explicitly enabled dedicated user clients and native
  transport/model/personalization checks; [prerequisites](../reference/telegram-acceptance.md).
  No actual sends were executed during implementation.
- `make test-e2e-live`: opt-in scoped read-only native snapshot, optional same-Pod
  SIGTERM; required environment and safety scope in [compatibility](../reference/compatibility.md).

Never construct upstream SessionStore from a separate exec process to assert a
running gateway's state. Its health publishing changes gateway PID identity and
can create false liveness failures. Native fixture creation before immediate
controlled shutdown is valid; post-start assertions use read-only SQLite/JSON.

## Local Task 8 execution

Final run passed in 156.90 seconds: three local subtests passed, Telegram explicitly
skipped; temporary kind cluster deleted successfully. Evidence directory:
`/tmp/hermes-e2e-task8-final`; initial source `ec054ff`, task7 local operator image
`sha256:9128baf96bbedba277ec4904809807bfc5b38a6154674a5bd99f58644893518f`.
Helm install, explicit CRD/Helm upgrade, manager restart, two-namespace lifecycle,
CR/used-Secret updates, actual Pod UID replacement, PVC marker retention,
same-name adoption rejection, existingClaim attachment, Delete and uninstall
checks passed. Pending CR lifecycle success is not native gateway Ready acceptance.
Run 1 failed its fixture DNS settings: resolver Service IP without DNS Pod selector
was insufficient after Calico DNAT. The harness now explicitly configures both;
no product code changed. Public IPv6 failed unrestricted baseline and is not-run.

`acceptance.json` contains every A01–A12 key for this local run only; it intentionally
does not import prior Berger passes. `tests.txt`, `verdict.json`, `lifecycle.json`
and `storage.json` are the corresponding executed evidence.

## Review fix: controlled metadata path and executable Telegram clients

`/tmp/hermes-e2e-review1-dnat` passed in 128.57 seconds (three local groups), with
explicit Telegram skips and successful rule/cluster cleanup. Both clients reached
`169.254.169.254:18080` before isolation; the unrestricted control remained reachable
while the selected client timed out after deny, exact unrelated exception and
restoration. `metadata-fixture.json` records exact owned container ID, two source
Pod IPs, controlled backend and successful cleanup. This is an owned-kind-node
post-DNAT path, not a real cloud metadata exposure test. The initial proposed
Service externalIPs fixture was rejected by API admission and never probed;
`/tmp/hermes-e2e-review1` preserves that failed setup and cleanup.

The same run compared the original CR UID across actual Helm upgrade. Every local
A01–A12 row now contains scope and evidence paths. Stronger live snapshots hash
actual history content, unmanaged personal configuration and cron jobs, excluding
operational ticker timestamps; offline fixture tests detect same-count message
edits and personal/cron changes. The enhanced read-only native loader and state snapshot check also passed on Berger
(`/tmp/hermes-e2e-live-enhanced/live-snapshot.json`, Pod UID
`c6b47bf9-2a8a-48ac-a3f5-febc51dce575`); see [build verification](build-verification.md).
That scoped check did not restart the agent or send Telegram messages and does
not close A02/A03 or the remaining release qualification gates.

An executable dedicated Telethon workflow now covers DM, first contact,
personalization, unauthorized identity, denied group text/command/media and
optional allowed group. It requires explicit send opt-in, verified dedicated
identities and actual transport/native evidence; no pass is inferred from a human
checkbox. Seven offline harness tests passed, including no-send on wrong identity
and rejection of silence without delivery/control evidence. No Telegram sending
was executed at that implementation checkpoint. The subsequent bounded manual
run closes A02; A03 remains `not-run`. See [workflow](../reference/telegram-acceptance.md)
and [fresh smoke](fresh-smoke-2026-09-21.md).
