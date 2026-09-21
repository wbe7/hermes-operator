#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export E2E_DIR="${E2E_DIR:-$(mktemp -d /tmp/hermes-e2e.XXXXXX)}"
mkdir -p "$E2E_DIR"
chmod 700 "$E2E_DIR"
export KUBECONFIG="$E2E_DIR/kubeconfig"
export E2E_CONTEXT=kind-hermes-operator-e2e
kind="${KIND:-kind}"
for tool in "$kind" kubectl helm docker python3; do command -v "$tool" >/dev/null; done
[[ "$($kind version)" == *v0.33.0* ]] || { echo 'kind v0.33.0 required'; exit 1; }
if "$kind" get clusters | grep -qx hermes-operator-e2e; then
  echo 'Refusing to reuse/delete a pre-existing hermes-operator-e2e cluster'; exit 1
fi
cleanup() {
  local rc=$?
  trap - EXIT
  "$kind" delete cluster --name hermes-operator-e2e
  echo "Evidence: $E2E_DIR (exit $rc)"
  exit "$rc"
}
trap cleanup EXIT
"$kind" create cluster --name hermes-operator-e2e --kubeconfig "$KUBECONFIG" \
  --image kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed \
  --config test/e2e/fixtures/kind.yaml
[[ "$(kubectl config current-context)" == "$E2E_CONTEXT" ]] || exit 1
for entry in \
  v1_crd_projectcalico_org.yaml:cb46786692404fb7955c6d31329c667d0dd8f1ff4821dc7869d0156c93d3a859 \
  tigera-operator.yaml:4e372754e4e1c3a61cf7c0ceb9bc9fff2f78aa6575dcadcb5ea4e716bb00dffa; do
  file="${entry%%:*}"; hash="${entry#*:}"
  curl -fsSL "https://raw.githubusercontent.com/projectcalico/calico/v3.32.2/manifests/$file" -o "$E2E_DIR/$file"
  python3 -c 'import hashlib,sys; assert hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest()==sys.argv[2]' "$E2E_DIR/$file" "$hash"
  kubectl --context "$E2E_CONTEXT" apply --server-side -f "$E2E_DIR/$file"
done
kubectl --context "$E2E_CONTEXT" apply -f test/e2e/fixtures/calico-installation.yaml
kubectl --context "$E2E_CONTEXT" wait --for=condition=Ready nodes --all --timeout=8m
kubectl --context "$E2E_CONTEXT" -n calico-system rollout status daemonset/calico-node --timeout=5m
# The immutable source snapshot is recorded alongside the image identity.
git rev-parse HEAD > "$E2E_DIR/source-commit.txt"
if [[ -n "${E2E_OPERATOR_IMAGE:-}" ]]; then
  docker tag "$E2E_OPERATOR_IMAGE" hermes-operator:e2e
else
  docker build -t hermes-operator:e2e .
fi
docker image inspect hermes-operator:e2e --format '{{.Id}}' > "$E2E_DIR/operator-image.txt"
"$kind" load docker-image hermes-operator:e2e --name hermes-operator-e2e
python3 test/e2e/fixtures/acceptance.py install
E2E_ENABLED=1 GOTOOLCHAIN=go1.27.1 GOMODCACHE=/tmp/hermes-go-mod GOCACHE=/tmp/hermes-go-cache \
  go test -count=1 -timeout=20m -v ./test/e2e | tee "$E2E_DIR/tests.txt"
