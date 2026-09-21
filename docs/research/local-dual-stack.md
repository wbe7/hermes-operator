# Native dual-stack policy verification

Observed 2026-09-21. These results verify the generated NetworkPolicy on a real CNI; they do not substitute for controller lifecycle, Telegram conversation or release-image acceptance.

## Environment and method

The root-created `hermes-operator-e2e-preflight` cluster was recreated with native dual-stack networking. The earlier IPv4 evidence in [local-e2e-preflight.md](local-e2e-preflight.md) remains historical; its temporary kubeconfig is obsolete. The new explicit kubeconfig is `/tmp/hermes-e2e-dual/kubeconfig`. No global context, Berger cluster networking or unrelated Docker container was changed.

The two arm64 nodes use kind v0.33.0 and Kubernetes v1.36.4, image `kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed`. Calico v3.32.2 uses VXLAN, IPv6 autodetection and outgoing NAT. The exact official manifest hashes recorded in the earlier preflight were verified again. Calico, API server, IP pools and tiers all reached Available=True / Degraded=False.

Configured Pod networks are `10.244.0.0/16` and `fd00:10:244::/56`; Service networks are `10.96.0.0/16` and `fd00:10:96::/112`. Node networks are `172.27.0.0/16` and `fc00:f853:ccd:e793::/64`. The test-only CoreDNS Service was made dual-stack. These are fixture settings, not installation defaults.

Four rootless, restricted Python fixtures used the pinned image from the earlier preflight. Control and isolated clients ran on the control-plane node; the target and its neighbor ran on the worker. The targets listened on TCP 8080/8081 for both address families. A suspended real Hermes CR supplied the installation UID; the policy was compiled by the product network builder, not handwritten. Both DNS pod selection and exact resolver IPs were explicit installation inputs.

Before isolation, both clients had to reach each controlled private endpoint. The test then applied the generated policy, opened exact target addresses on TCP 8080, and restored the original policy in `finally`. Assertions required a continuing unrestricted positive control. Known node APIs were tested only with TCP connect/close; no credentials, HTTP calls or network scanning were used.

## Results

| Check | IPv4 | IPv6 |
| --- | --- | --- |
| Unrestricted private Pod, Service and hosting-node baseline | Passed | Passed |
| Generated policy denies target ports, neighbor, Service and hosting-node API | Passed | Passed |
| Configured DNS, TCP and UDP 53 | Passed | Passed |
| Exact target IP + TCP 8080 exception | Passed | Passed |
| Other port, neighbor and hosting-node API remain denied | Passed | Passed |
| Restored policy denies target again | Passed | Passed |
| Public TCP 443 and UDP 53 | Passed | Unavailable before policy |

The Service translating to the explicitly allowed backend/port also became reachable in both families. This is the observed Calico DNAT behavior; the exception does not preserve an independent pre-NAT Service-address denial.

External IPv6 could not be verified: unrestricted probes to `2606:4700:4700::1111` already failed (TCP refused, UDP timeout). This is not evidence that the policy denied public IPv6. Native IPv6 Pod, Service, node, DNS and exact-exception checks did execute successfully; external IPv6 routing remains an environment limitation.

The final original deny policy was restored and checked. No real Telegram credentials or messages were used. Local evidence under `/tmp/hermes-e2e-dual/`: `kind.yaml`, `installation.yaml`, `prepare-fixtures.py`, `check-network.py`, `run-policy-checks.py`, `network-config.json`, `policy.json`, `policy-allow.json`, `baseline.json`, `isolated.json`, `exception.json`, `restored.json` and `verdict.json`. Temporary artifacts are a local run record; the committed end-to-end harness is the reproducibility gate.
