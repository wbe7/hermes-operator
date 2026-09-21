# Local reference-cluster preflight

Observed 2026-09-18. This is environment and compiler integration evidence; it does not by itself prove controller lifecycle or a release compatibility matrix.

## Pinned environment

- kind v0.33.0, macOS arm64 CLI in `/tmp/hermes-e2e-tools/kind`; official binary SHA256 `0c8c7dbe5e23594a198b786c4bc13dacc101fa6196b0cb0b23a1ca44e61f4b4f` verified before execution.
- Docker Desktop: 8 CPUs, approximately 16 GiB memory. Separate empty Docker client config avoided the existing credential helper hang.
- Kubernetes v1.36.4, two arm64 nodes, Linux 6.12.76-linuxkit; official `kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed` from the [kind release](https://github.com/kubernetes-sigs/kind/releases/tag/v0.33.0).
- Calico v3.32.2, operator installation, VXLAN, BGP disabled. The [Calico requirements](https://docs.tigera.io/calico/latest/getting-started/kubernetes/requirements) list testing against Kubernetes 1.34–1.36; Kubernetes 1.37 is not inferred from this.
- Following the [official kind installation](https://docs.tigera.io/calico/latest/getting-started/kubernetes/kind), default kind CNI disabled; pod CIDR `10.244.0.0/16`, service CIDR `10.96.0.0/16`. Only Installation and APIServer enabled; no optional flow aggregator/UI.

Downloaded official versioned manifests:

| File | SHA256 |
| --- | --- |
| `v1_crd_projectcalico_org.yaml` | `cb46786692404fb7955c6d31329c667d0dd8f1ff4821dc7869d0156c93d3a859` |
| `tigera-operator.yaml` | `4e372754e4e1c3a61cf7c0ceb9bc9fff2f78aa6575dcadcb5ea4e716bb00dffa` |
| `custom-resources.yaml` (read for API shape; not applied wholesale) | `4fb598a7fce1ad1f8bbfb9004f5efcdb835082e92e06323cb7b8ce60260ed891` |

All are under `https://raw.githubusercontent.com/projectcalico/calico/v3.32.2/manifests/`. Local files and kubeconfig are under `/tmp/hermes-e2e-preflight`. Cluster name `hermes-operator-e2e-preflight`; no global kubeconfig switch or Berger CNI change. Both nodes reached Ready; Calico, API server, IP pools and tiers reported Available=True / Degraded=False.

## API and controlled endpoints

The committed Hermes CRD installed and Established. All three design examples passed server-side dry-run on Kubernetes 1.36.4. A suspended `network-smoke` CR provides a real UID for policy ownership/selector tests; it has no real credentials or running Hermes workload.

Four restricted rootless Python fixture Pods use image `python@sha256:c4634f578a412db396771b61b064c6e546c9d6414c7fb5b1b05d5871f1885f7b`. Control and isolated clients run on the control-plane node; target and neighboring target run on the worker. Targets serve fixed `ok` responses on TCP 8080/8081. No host mounts, capabilities or service-account tokens.

Before applying restrictive policy, both clients reached:

- target `10.244.192.131` on TCP 8080 and 8081;
- neighboring target `10.244.192.132:8080`;
- target Service `10.96.230.243:8080`;
- their hosting-node API `172.27.0.3:6443` (TCP connect/close only);
- DNS `10.96.0.10` on UDP/TCP 53;
- public Telegram TCP 443 and a public UDP DNS query for `example.com` to `1.1.1.1:53`.

The script `/tmp/hermes-e2e-preflight/check-network.py` and `network-baseline.json` contain the bounded check and result. Addresses are observations, not reusable install defaults. No Telegram API message or user data was sent. Final generated-policy results must be recorded separately; baseline reachability alone does not prove isolation. Native dual-stack remains untested here.

## Generated policy on Calico

Compiler commit `90c5ce8` built the policy from the real suspended CR UID and validated installation inputs. The generated resource was accepted by Kubernetes. With no private exception, target ports 8080/8081, adjacent target, target Service and the hosting-node API all timed out from the selected client; the unrestricted control remained reachable. DNS UDP/TCP and public TCP/UDP remained available. These are actual CNI results, not the unit matcher.

The first exact-exception assertion exposed an incorrect test assumption: allowing the target Pod IP on TCP 8080 also permits the Service that DNATs to that same backend/port. Calico evaluates this path against the translated destination. Target port 8081 and the adjacent target remained blocked. This matches the documented NAT boundary; it is not an additional destination beyond that backend. The test expectation was corrected explicitly, with policy restoration in `finally`.

The corrected exception test passed: only target TCP 8080 and its DNAT Service opened; target TCP 8081, adjacent Pod and node API remained denied. After restoring the original generated policy, target TCP 8080 was denied again. Assertions also required uninterrupted positive control, DNS and public TCP/UDP. Evidence: `network-exception.json`, `network-restored.json` and `check-exception.py` under the local preflight directory.
