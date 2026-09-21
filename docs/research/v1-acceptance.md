# v1 acceptance ledger

Status: **not release-ready**. A row passes only when all its required subchecks
pass. The executable local suite deliberately does not turn synthetic fixture
success into native Hermes or Telegram acceptance.

| ID | Status | Executed evidence / remaining gate |
| --- | --- | --- |
| A01 | not-run (partial passed) | Berger controller reconciled separate namespaces and claims; local command `make test-e2e` checks two namespace identities. Two simultaneously Ready native installations with distinct dedicated credentials remain unverified. |
| A02 | not-run | No real allowed-user DM or first-contact personalization observed. Requires dedicated human test identity, actual response, retained user preferences and SOUL/personality changes. |
| A03 | not-run | Offline upstream filter tests exist (`make test-runtime`); actual unauthorized sender/group text/commands/media and accepted old-inline-callback limitation require dedicated Telegram identities. |
| A04 | passed | Berger original gateway same-Pod SIGTERM, restart count +1, native model/provider/credential/xhigh restoration, original session ID/history. Corrected read-only verifier and 25 health samples: `/tmp/hermes-operator-deploy/health-restart-samples.json`, `verify-builder-state.py`; [detailed record](berger-apps-preflight.md). |
| A05 | passed | Controller-managed replacement preserved original PVC/session and 333 personal file hashes. `controller-revocation.json`, corrected `verify-builder-state.py`, [detailed record](berger-apps-preflight.md). Real Telegram-created conversation not yet covered by this synthetic native session. |
| A06 | passed | Real controller extraConfig add/remove produced new immutable inputs; original-image restore-only on the same actual PVC removed the previously managed compression threshold while preserving adjacent personal config and SOUL. `/tmp/hermes-operator-deploy/extra-removal.json`; no Telegram consumer needed for this bootstrap check. |
| A07 | passed | Actual Berger created Retain/Delete/existing claims, marker preserved across expansion, no automatic adoption; `/tmp/hermes-operator-deploy/storage-lifecycle.json`. Local repeat also checks persistence after Helm uninstall. |
| A08 | passed | Deleted/missing Secret and unsupported version confirmed by `controller-revocation.json`; offline corruption tests exist. Berger disposable original-image fixture proved corrupt YAML/SQLite and PVC permissions fail closed without reset and recover after repair using restore-only; `corrupt-home.json`. This does not claim fake-token Telegram Ready. Actual Telegram token conflict remains not-run (no second real-token consumer). |
| A09 | not-run (partial passed) | Original-image UID10000/CapEff0/NoNewPrivs/no SA mount; generated policy on actual Berger Pod; reference Calico native dual-stack DNS/private/Service/node/exact exceptions. [Local evidence](local-dual-stack.md). External publicIPv6 unavailable in unrestricted baseline; metadata stand-in untested. |
| A10 | passed | Controller suspend/revocation/resume on preserved PVC; manager SIGTERM preserved gateway Pod UID and revision (`manager-restart.json`). Telegram cold-start outage recovered without restart storm; `telegram-outage-results.json`. Actual provider IP exception removed for 100 seconds: six idle health samples remained live/ready with zero restarts and same Pod/revision; rule restored and TCP recovered (`provider-outage.json`). No active user turn was simulated. |
| A11 | passed | Berger CSI WaitForFirstConsumer + actual 1Gi→2Gi expansion retained marker; shrink admission rejection in schema tests. `storage-lifecycle.json`. Local kind storage class does not advertise expansion; no local expansion claim. |
| A12 | not-run (local passed) | `make test-e2e` installs real Helm chart, explicitly applies CRD before upgrade, checks active Pending CR survival, deletes CR normally, uninstalls, checks retained PVC UID. Local install/upgrade/uninstall passed with active Pending CR; Ready native gateway across chart upgrade remains unverified. No automatic data-schema rollback claim. |

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
