GO ?= go
GOFLAGS_ENV := GOTOOLCHAIN=go1.27.1 GOMODCACHE=/tmp/hermes-go-mod GOCACHE=/tmp/hermes-go-cache
LOCALBIN := $(CURDIR)/bin
CONTROLLER_GEN_VERSION := v0.22.0
CONTROLLER_GEN := $(LOCALBIN)/controller-gen-$(CONTROLLER_GEN_VERSION)
SETUP_ENVTEST_VERSION := v0.0.0-20260125163108-a19ec76a3c5d
SETUP_ENVTEST := $(LOCALBIN)/setup-envtest-$(SETUP_ENVTEST_VERSION)
ENVTEST_K8S_VERSION := 1.34.1

.PHONY: generate manifests test-unit test-envtest test-runtime verify-generated tools

tools: $(CONTROLLER_GEN) $(SETUP_ENVTEST)

$(LOCALBIN):
	mkdir -p $@

$(CONTROLLER_GEN): | $(LOCALBIN)
	$(GOFLAGS_ENV) GOBIN=$(LOCALBIN) $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	mv $(LOCALBIN)/controller-gen $@

$(SETUP_ENVTEST): | $(LOCALBIN)
	$(GOFLAGS_ENV) GOBIN=$(LOCALBIN) $(GO) install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)
	mv $(LOCALBIN)/setup-envtest $@

generate: generate-runtime-assets $(CONTROLLER_GEN)
	$(GOFLAGS_ENV) $(CONTROLLER_GEN) object:headerFile= paths=./api/...

manifests: $(CONTROLLER_GEN)
	$(GOFLAGS_ENV) $(CONTROLLER_GEN) crd:crdVersions=v1 rbac:roleName=hermes-operator paths=./... output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac

test-unit:
	$(GOFLAGS_ENV) $(GO) test ./internal/...

test-envtest: $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GOFLAGS_ENV) $(GO) test -count=1 ./api/v1alpha1 ./internal/workload ./internal/controller

test-runtime:
	test/runtime/run.sh

verify-generated: verify-runtime-assets $(CONTROLLER_GEN)
	@set -eu; before=$$(mktemp); after=$$(mktemp); \
	trap 'rm -f "$$before" "$$after"' EXIT; \
	find api -name 'zz_generated.deepcopy.go' -type f -print; \
	find api -name 'zz_generated.deepcopy.go' -type f -print0 | sort -z | xargs -0 shasum > "$$before"; \
	find config -type f -print0 | sort -z | xargs -0 shasum >> "$$before"; \
	$(MAKE) generate manifests >/dev/null; \
	find api -name 'zz_generated.deepcopy.go' -type f -print0 | sort -z | xargs -0 shasum > "$$after"; \
	find config -type f -print0 | sort -z | xargs -0 shasum >> "$$after"; \
	diff -u "$$before" "$$after"

.PHONY: generate-runtime-assets verify-runtime-assets
generate-runtime-assets:
	python3 internal/workload/generate_assets.py

verify-runtime-assets:
	python3 internal/workload/generate_assets.py --check
