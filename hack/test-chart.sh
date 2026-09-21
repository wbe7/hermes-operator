#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
chart="$repo_root/charts/hermes-operator"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

expect_failure() {
  local name=$1
  local values=$2
  if helm template "$name" "$chart" --kube-version 1.34.1 --namespace hermes-system --values "$values" >"$tmp/$name.out" 2>"$tmp/$name.err"; then
    echo "expected Helm rendering to reject $name" >&2
    return 1
  fi
}

cat >"$tmp/missing-confirmation.yaml" <<'EOF'
networkPolicy:
  podCIDRs: [10.244.0.0/16]
  serviceCIDRs: [10.96.0.0/12]
  nodeCIDRs: [192.168.1.0/24]
  infrastructureCIDRs: []
  dns:
    resolverIPs: [10.96.0.10]
EOF
expect_failure missing-confirmation "$tmp/missing-confirmation.yaml"

cat >"$tmp/missing-cidrs.yaml" <<'EOF'
networkPolicy:
  enforcementConfirmed: true
  dns:
    resolverIPs: [10.96.0.10]
EOF
expect_failure missing-cidrs "$tmp/missing-cidrs.yaml"

cat >"$tmp/missing-dns.yaml" <<'EOF'
networkPolicy:
  enforcementConfirmed: true
  podCIDRs: [10.244.0.0/16]
  serviceCIDRs: [10.96.0.0/12]
  nodeCIDRs: [192.168.1.0/24]
  infrastructureCIDRs: []
EOF
expect_failure missing-dns "$tmp/missing-dns.yaml"

cat >"$tmp/ipv4.yaml" <<'EOF'
networkPolicy:
  enforcementConfirmed: true
  podCIDRs: [10.244.0.0/16]
  serviceCIDRs: [10.96.0.0/12]
  nodeCIDRs: [192.168.1.0/24]
  infrastructureCIDRs: []
  dns:
    podSelector:
      namespaceLabels:
        kubernetes.io/metadata.name: kube-system
      podLabels:
        k8s-app: kube-dns
    resolverIPs: [10.96.0.10]
EOF
helm template ipv4 "$chart" --kube-version 1.34.1 --namespace hermes-system --values "$tmp/ipv4.yaml" >"$tmp/ipv4.out"

cat >"$tmp/dual-stack.yaml" <<'EOF'
networkPolicy:
  enforcementConfirmed: true
  podCIDRs: [10.244.0.0/16, fd00:10:244::/56]
  serviceCIDRs: [10.96.0.0/12, fd00:10:96::/112]
  nodeCIDRs: [192.168.1.0/24, fd00:192:168::/64]
  infrastructureCIDRs: []
  dns:
    resolverIPs: [10.96.0.10, fd00:10:96::a]
EOF
helm template dual-stack "$chart" --kube-version 1.34.1 --namespace hermes-system --values "$tmp/dual-stack.yaml" >"$tmp/dual-stack.out"

for rendered in "$tmp/ipv4.out" "$tmp/dual-stack.out"; do
  grep -q 'kind: Deployment' "$rendered"
  grep -q 'runAsNonRoot: true' "$rendered"
  grep -q 'readOnlyRootFilesystem: true' "$rendered"
  grep -q 'seccompProfile:' "$rendered"
  grep -q 'drop:' "$rendered"
  grep -q -- '- ALL' "$rendered"
  grep -q 'kind: ClusterRole' "$rendered"
  grep -q 'kind: Role' "$rendered"
  grep -q 'kind: ConfigMap' "$rendered"
  grep -q 'POD_NAMESPACE' "$rendered"
  if grep -q '^kind: Service$' "$rendered"; then
    echo "operator chart must not publish a Service" >&2
    exit 1
  fi
done

grep -q 'infrastructureCIDRs: \[\]' "$tmp/ipv4.out"
grep -q 'resolverIPs:' "$tmp/ipv4.out"
grep -q 'podSelector:' "$tmp/ipv4.out"
grep -q 'fd00:10:244::/56' "$tmp/dual-stack.out"
helm lint "$chart" --kube-version 1.34.1 --values "$tmp/ipv4.yaml"

helm package "$chart" --version 7.8.9 --app-version 7.8.9 --destination "$tmp" >/dev/null
helm template packaged "$tmp/hermes-operator-7.8.9.tgz" \
  --kube-version 1.34.1 --namespace hermes-system --values "$tmp/ipv4.yaml" \
  >"$tmp/packaged.out"
grep -q 'image: "ghcr.io/wbe7/hermes-operator:7.8.9"' "$tmp/packaged.out"
