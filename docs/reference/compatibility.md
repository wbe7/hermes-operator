# Verified compatibility boundaries

This is an evidence matrix, not a release support promise. The mandatory v1
acceptance gate remains open; see [acceptance](../research/v1-acceptance.md).

| Environment | Architecture | Verified scope |
| --- | --- | --- |
| k3s v1.34.7+k3s1, Flannel + k3s policy enforcement | amd64 | Native original gateway, controller source build, original-image startup restore, CSI WaitForFirstConsumer/expansion, generated policy. [Evidence](../research/berger-apps-preflight.md). |
| kind v0.33.0 / Kubernetes v1.36.4 / Calico v3.32.2 | arm64 | Native dual-stack CNI probes; original-image runtime tests. Local Helm acceptance command below; run status in acceptance report. |

No Kubernetes 1.35 or 1.37 runtime support is claimed. CI's amd64 reference-cluster
job is executable coverage, not proof of a completed CI run. The Helm chart is
built from the current source snapshot (chart version 0.1.0); record the source
commit and actual image ID, not an unpublished release tag.

Pinned original Hermes: `v2026.9.14`, multiarch image digest
`sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294`.
The kind node and fixture images are digest-pinned in `hack/e2e-cluster.sh` and
`test/e2e/fixtures/acceptance.py`; Calico artifacts have embedded SHA256 checks.
Helm CLI used locally/CI is v3.15.0.

Run `make test-e2e` with Docker, kind v0.33.0, kubectl, Helm and Python 3 available.
The command creates/deletes only `hermes-operator-e2e`, refuses an existing cluster
of that name, uses a temporary kubeconfig and never changes the global context.
`E2E_DIR` selects retained evidence. `KIND` selects the CLI path.
`E2E_OPERATOR_IMAGE` optionally selects an existing locally built image; its image
ID and current source commit are recorded, but the harness cannot prove the
selected image was built from that commit. Omit it for a fresh source build.

The generated policy is tested over native IPv4 and IPv6 Pod/Service/node paths,
DNS TCP/UDP, exact backend exceptions and public IPv4. Public IPv6 is required
only when the unrestricted baseline has connectivity; unavailable baseline is
recorded as not-run. Calico DNAT can allow a Service mapping to an allowed backend.
Hosting-node probes prove only the known API listener, not all possible host paths.
Metadata is not contacted; a dedicated metadata-address stand-in remains untested.

Live read-only snapshots use `make test-e2e-live`, with `E2E_LIVE_KUBECONFIG`,
`E2E_LIVE_CONTEXT`, `E2E_LIVE_NAMESPACE` (prefix `hermes-operator-test`),
`E2E_LIVE_HERMES`, `E2E_LIVE_DEDICATED=yes`, and `E2E_DIR`.
Credentials must already be provisioned via a namespace-local Secret using
protected environment/temporary files, never literals or logs. The harness does
not retrieve Secret objects. Set `E2E_LIVE_RESTART=yes` only for an explicitly
authorized same-Pod SIGTERM. It checks native restored configuration and read-only
SQLite/file snapshots without instantiating SessionStore against a live gateway.
It does not send Telegram messages or run a second token consumer.
