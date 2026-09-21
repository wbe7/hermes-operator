GO ?= go
GOFLAGS_ENV := GOTOOLCHAIN=go1.27.1 GOMODCACHE=/tmp/hermes-go-mod GOCACHE=/tmp/hermes-go-cache
LOCALBIN := $(CURDIR)/bin
CONTROLLER_GEN_VERSION := v0.22.0
CONTROLLER_GEN := $(LOCALBIN)/controller-gen-$(CONTROLLER_GEN_VERSION)
SETUP_ENVTEST_VERSION := v0.0.0-20260125163108-a19ec76a3c5d
SETUP_ENVTEST := $(LOCALBIN)/setup-envtest-$(SETUP_ENVTEST_VERSION)
ENVTEST_K8S_VERSION := 1.34.1

.PHONY: generate manifests test-unit test-envtest test-runtime lint-chart verify-docs verify-generated tools

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
	CONTROLLER_GEN=$(CONTROLLER_GEN) hack/verify-generated.sh

lint-chart:
	hack/test-chart.sh

verify-docs:
	python3 hack/verify-docs.py

.PHONY: generate-runtime-assets verify-runtime-assets
generate-runtime-assets:
	python3 internal/workload/generate_assets.py

verify-runtime-assets:
	python3 internal/workload/generate_assets.py --check

.PHONY: test-e2e test-e2e-live
test-e2e:
	hack/e2e-cluster.sh

test-e2e-live:
	python3 hack/e2e-live.py

.PHONY: test-e2e-telegram test-e2e-harness
test-e2e-telegram:
	$${E2E_TELEGRAM_PYTHON:-python3} hack/e2e-telegram.py

test-e2e-harness:
	python3 -m unittest discover -s test/e2e/fixtures -p 'test_*.py'
