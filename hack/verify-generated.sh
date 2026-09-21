#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
controller_gen=${CONTROLLER_GEN:-"$repo_root/bin/controller-gen-v0.22.0"}
export GOTOOLCHAIN=${GOTOOLCHAIN:-go1.27.1}
export GOMODCACHE=${GOMODCACHE:-/tmp/hermes-go-mod}
export GOCACHE=${GOCACHE:-/tmp/hermes-go-cache}
if [[ ! -x "$controller_gen" ]]; then
  echo "controller-gen not found at $controller_gen; run make tools" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/repo"
cp "$repo_root/go.mod" "$repo_root/go.sum" "$tmp/repo/"
cp -R "$repo_root/api" "$repo_root/cmd" "$repo_root/config" "$repo_root/internal" "$repo_root/runtime" "$tmp/repo/"

# Generate into clean destinations so stale extra files cannot survive and
# compare equal merely because they were copied from the source tree.
rm -f "$tmp/repo/api/v1alpha1/zz_generated.deepcopy.go"
rm -rf "$tmp/repo/config/crd/bases" "$tmp/repo/internal/workload/assets"
rm -f "$tmp/repo/config/rbac/role.yaml"
mkdir -p "$tmp/repo/config/crd/bases" "$tmp/repo/internal/workload/assets"

(
  cd "$tmp/repo"
  python3 internal/workload/generate_assets.py
  "$controller_gen" object:headerFile= paths=./api/...
  "$controller_gen" crd:crdVersions=v1 rbac:roleName=hermes-operator paths=./... \
    output:crd:artifacts:config=config/crd/bases \
    output:rbac:artifacts:config=config/rbac
)

diff -ru "$repo_root/api" "$tmp/repo/api"
diff -ru "$repo_root/config" "$tmp/repo/config"
diff -ru "$repo_root/internal/workload/assets" "$tmp/repo/internal/workload/assets"
cmp "$repo_root/config/crd/bases/hermes.wbe7.github.io_hermes.yaml" \
  "$repo_root/charts/hermes-operator/crds/hermes.wbe7.github.io_hermes.yaml"
cmp "$repo_root/config/rbac/role.yaml" "$repo_root/charts/hermes-operator/files/role.yaml"
cmp "$repo_root/config/rbac/leader-election-role.yaml" \
  "$repo_root/charts/hermes-operator/files/leader-election-role.yaml"
