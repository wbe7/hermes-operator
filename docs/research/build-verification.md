# Operator build verification

Observed 2026-09-21; this record describes the tested artifacts and does not certify future builds or the upstream Hermes image.

- `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...`, using Go 1.27.1: exit 0, `No vulnerabilities found.`
- The local Task 7 arm64 operator image was built and started successfully with UID/GID 65532 and `/hermes-operator` as entrypoint. Image ID: `sha256:9128baf96bbedba277ec4904809807bfc5b38a6154674a5bd99f58644893518f`. The real kind/Helm tests subsequently ran this image; native amd64 source execution is recorded separately in [Berger evidence](berger-apps-preflight.md).
- Trivy 0.74.0, image digest `sha256:62b1e65e8869bc4b4c6aa4fa2b21595256c7c2f6018a9d9ad61caf87187c1969`, scanned an exported archive of that operator image. `--scanners vuln --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1` exited 0: no matching findings in Debian 12.15 packages or the Go binary. This is a filter for fixable high/critical findings, not a claim about every severity or future database contents.

The scanner ran as UID/GID 65532, with dropped capabilities, no privilege escalation, read-only root and access only to the read-only image archive and a dedicated cache. An initial Docker-socket invocation was rejected before execution; the archive-based scan completed without daemon access. Local run details are in `/tmp/hermes-image-audit/scan-summary.txt`.

The requested development `.env` has mode 0600, is ignored by Git and is not tracked. An in-memory comparison found neither supplied development credential, nor its base64 encoding, in tracked files or local Git patch history. Values were never emitted by the check. Old local test containers and temporary credential homes were removed after read-only SQLite checks confirmed zero sessions/messages; the requested `.env` and active test installation remain.

No image, chart, Git branch or release was published by these checks. For current platform coverage and remaining gates, see [compatibility](../reference/compatibility.md) and [acceptance](v1-acceptance.md).

The strengthened `make test-e2e-live` snapshot at Task 8 fix `f4e3a55` also ran successfully against the real Berger gateway in read-only mode. It compared the native selected credential inside the Pod and collected history-content, personal-config, cron and file hashes without changing gateway PID identity. `/tmp/hermes-e2e-live-enhanced/live-snapshot.json` records the scoped pass; the private state snapshot is not a release attachment. This invocation did not restart the agent or send Telegram messages.

## Reviewed implementation progress

Tasks 1–7 have implemented and reviewed API, configuration restoration, network/workload construction, reconciliation and packaging. Task 8 has an executable acceptance harness and recorded scoped local/ Berger results; full release qualification remains open in the [acceptance ledger](v1-acceptance.md). The original plan checklist is historical, not evidence that unexecuted gates passed.

The consolidated F1–F8 review fixes were checked with Kubernetes 1.34.1 envtest API/controller/config/workload suites and 23 tests in the pinned original Hermes image. Covering regressions verify applied-source revocation on apply/preflight failures, corrupt dotenv byte retention and repair, live Pod identity/isolation drift, Service-safe CR names, isolated positive-limit admission, controlled authorization diagnostics and deprecated cwd cleanup. Native loading and the terminal config accessor resolve cwd to /opt/data/workspace without deprecated dotenv entries. Generated assets/schema and documentation checks pass. This source validation does not imply deployment of the fixes or close the remaining live release gates.
