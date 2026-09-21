package v1alpha1_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

var hermesGVR = schema.GroupVersionResource{Group: "hermes.wbe7.github.io", Version: "v1alpha1", Resource: "hermes"}

func TestHermesAdmission(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join("..", "..")
	env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join(root, "config", "crd", "bases")}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })

	dynamicClient, err := newDynamicClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dynamicClient.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}).Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "schema-test"}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	minimalBytes, err := os.ReadFile(filepath.Join(root, "internal", "testfixtures", "minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if err := yaml.Unmarshal(minimalBytes, &base); err != nil {
		t.Fatal(err)
	}
	base["metadata"].(map[string]any)["namespace"] = "schema-test"

	create := func(t *testing.T, name string, mutate func(map[string]any), strict bool) error {
		t.Helper()
		b, _ := json.Marshal(base)
		var obj map[string]any
		_ = json.Unmarshal(b, &obj)
		obj["metadata"].(map[string]any)["name"] = name
		mutate(obj)
		opts := metav1.CreateOptions{}
		if strict {
			opts.FieldValidation = metav1.FieldValidationStrict
		}
		_, err := dynamicClient.Resource(hermesGVR).Namespace("schema-test").Create(ctx, &unstructured.Unstructured{Object: obj}, opts)
		return err
	}

	if err := create(t, "valid", func(map[string]any) {}, true); err != nil {
		t.Fatalf("minimal CR rejected: %v", err)
	}
	defaulted, err := dynamicClient.Resource(hermesGVR).Namespace("schema-test").Get(ctx, "valid", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]any{
		"spec.image.repository":          "docker.io/nousresearch/hermes-agent",
		"spec.model.auth":                "APIKey",
		"spec.reasoning.effort":          "xhigh",
		"spec.telegram.groups.enabled":   false,
		"spec.agent.maxTurns":            int64(50),
		"spec.agent.runBudgetSeconds":    int64(600),
		"spec.terminal.timeoutSeconds":   int64(300),
		"spec.resources.requests.cpu":    "100m",
		"spec.resources.limits.memory":   "2Gi",
		"spec.storage.deletionPolicy":    "Retain",
		"spec.storage.create.accessMode": "ReadWriteOnce",
	}
	for path, want := range defaults {
		got, found, err := unstructured.NestedFieldNoCopy(defaulted.Object, strings.Split(path, ".")...)
		if err != nil || !found || !reflect.DeepEqual(got, want) {
			t.Errorf("default %s: got %#v found=%v err=%v; want %#v", path, got, found, err, want)
		}
	}
	cases := map[string]func(map[string]any){
		"both-storage-sources": func(o map[string]any) {
			o["spec"].(map[string]any)["storage"].(map[string]any)["existingClaim"] = "data"
		},
		"empty-sender-allowlist": func(o map[string]any) {
			o["spec"].(map[string]any)["telegram"].(map[string]any)["allowedUserIDs"] = []any{}
		},
		"delete-existing": func(o map[string]any) {
			s := o["spec"].(map[string]any)["storage"].(map[string]any)
			delete(s, "create")
			s["existingClaim"] = "data"
			s["deletionPolicy"] = "Delete"
		},
		"none-auth-built-in": func(o map[string]any) {
			m := o["spec"].(map[string]any)["model"].(map[string]any)
			m["provider"] = "anthropic"
			delete(m, "baseURL")
			m["auth"] = "None"
		},
		"none-auth-with-key": func(o map[string]any) {
			m := o["spec"].(map[string]any)["model"].(map[string]any)
			m["auth"] = "None"
			m["apiKeySecretRef"] = map[string]any{"key": "MODEL_API_KEY"}
		},
		"custom-without-base-url": func(o map[string]any) { delete(o["spec"].(map[string]any)["model"].(map[string]any), "baseURL") },
		"groups-without-ids": func(o map[string]any) {
			o["spec"].(map[string]any)["telegram"].(map[string]any)["groups"] = map[string]any{"enabled": true}
		},
		"cidr-private-exception": func(o map[string]any) {
			o["spec"].(map[string]any)["network"] = map[string]any{"allowPrivate": []any{map[string]any{"ip": "10.0.0.0/24"}}}
		},
		"invalid-blocked-cidr": func(o map[string]any) {
			o["spec"].(map[string]any)["network"] = map[string]any{"additionalBlockedCIDRs": []any{"not-a-cidr"}}
		},
		"request-above-limit": func(o map[string]any) {
			o["spec"].(map[string]any)["resources"] = map[string]any{"requests": map[string]any{"memory": "3Gi"}, "limits": map[string]any{"memory": "2Gi"}}
		},
		"zero-request": func(o map[string]any) {
			o["spec"].(map[string]any)["resources"] = map[string]any{"requests": map[string]any{"cpu": "0"}, "limits": map[string]any{"cpu": "1"}}
		},
		"negative-request": func(o map[string]any) {
			o["spec"].(map[string]any)["resources"] = map[string]any{"requests": map[string]any{"memory": "-1Mi"}, "limits": map[string]any{"memory": "2Gi"}}
		},
		"zero-limit": func(o map[string]any) {
			o["spec"].(map[string]any)["resources"] = map[string]any{"requests": map[string]any{"cpu": "100m", "memory": "512Mi"}, "limits": map[string]any{"example.com/device": "0"}}
		},
		"negative-limit": func(o map[string]any) {
			o["spec"].(map[string]any)["resources"] = map[string]any{"limits": map[string]any{"memory": "-1Mi"}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			err := create(t, name, mutate, true)
			if err == nil || !apierrors.IsInvalid(err) {
				t.Fatalf("expected Invalid, got %v", err)
			}
			if name == "zero-limit" {
				status := err.(apierrors.APIStatus).Status()
				found := false
				for _, cause := range status.Details.Causes {
					if cause.Field == "spec.resources" && strings.Contains(cause.Message, "resource limits must be positive") {
						found = true
					}
					if strings.Contains(cause.Message, "resource requests must be positive") {
						t.Fatalf("limit fixture also failed request validation: %v", err)
					}
				}
				if !found {
					t.Fatalf("positive-limit admission cause missing: %v", err)
				}
			}
		})
	}
	for _, name := range []string{"alice.team", "1alice", strings.Repeat("a", 41)} {
		if err := create(t, name, func(map[string]any) {}, true); !apierrors.IsInvalid(err) {
			t.Fatalf("unsafe derived Service name %q accepted: %v", name, err)
		}
	}
	for _, name := range []string{"a", "alice-team1", strings.Repeat("a", 40)} {
		if err := create(t, name, func(map[string]any) {}, true); err != nil {
			t.Fatalf("valid boundary name %q rejected: %v", name, err)
		}
	}
	strictCases := map[string]func(map[string]any){
		"cross-namespace-ref": func(o map[string]any) {
			o["spec"].(map[string]any)["model"].(map[string]any)["apiKeySecretRef"] = map[string]any{"name": "s", "key": "k", "namespace": "other"}
		},
		"unknown-field": func(o map[string]any) { o["spec"].(map[string]any)["surprise"] = true },
	}
	for name, mutate := range strictCases {
		t.Run(name, func(t *testing.T) {
			if err := create(t, name, mutate, true); err == nil {
				t.Fatal("strict unknown field was accepted")
			}
		})
	}

	// Delayed-binding existing PVCs are dependencies resolved by the controller, not admission.
	if err := create(t, "delayed-binding", func(o map[string]any) {
		s := o["spec"].(map[string]any)["storage"].(map[string]any)
		delete(s, "create")
		s["existingClaim"] = "not-yet-bound"
	}, true); err != nil {
		t.Fatalf("existing unbound claim rejected at admission: %v", err)
	}

	for i, filename := range []string{"hermes-minimal.yaml", "hermes-existing-pvc.yaml", "hermes-local-inference.yaml"} {
		t.Run(filename, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, "examples", filename))
			if err != nil {
				t.Fatal(err)
			}
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				t.Fatal(err)
			}
			obj["metadata"].(map[string]any)["namespace"] = "schema-test"
			obj["metadata"].(map[string]any)["name"] = fmt.Sprintf("example-%d", i)
			if _, err := dynamicClient.Resource(hermesGVR).Namespace("schema-test").Create(ctx, &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{FieldValidation: metav1.FieldValidationStrict}); err != nil {
				t.Fatalf("example rejected: %v", err)
			}
		})
	}

	// Storage source is immutable and requested storage cannot shrink.
	valid, err := dynamicClient.Resource(hermesGVR).Namespace("schema-test").Get(ctx, "valid", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	updated := valid.DeepCopy()
	storage, _, _ := unstructured.NestedMap(updated.Object, "spec", "storage")
	delete(storage, "create")
	storage["existingClaim"] = "other"
	_ = unstructured.SetNestedMap(updated.Object, storage, "spec", "storage")
	if _, err := dynamicClient.Resource(hermesGVR).Namespace("schema-test").Update(ctx, updated, metav1.UpdateOptions{}); err == nil || !apierrors.IsInvalid(err) {
		t.Fatalf("storage source update: expected Invalid, got %v", err)
	}

	valid, _ = dynamicClient.Resource(hermesGVR).Namespace("schema-test").Get(ctx, "valid", metav1.GetOptions{})
	updated = valid.DeepCopy()
	_ = unstructured.SetNestedField(updated.Object, "1Gi", "spec", "storage", "create", "size")
	if _, err := dynamicClient.Resource(hermesGVR).Namespace("schema-test").Update(ctx, updated, metav1.UpdateOptions{}); err == nil || !apierrors.IsInvalid(err) {
		t.Fatalf("storage shrink: expected Invalid, got %v", err)
	}
}

func TestCRDIsStructuralAndHasStatus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "hermes.wbe7.github.io_hermes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(data, &crd); err != nil {
		t.Fatal(err)
	}
	if crd.Spec.Scope != apiextensionsv1.NamespaceScoped {
		t.Fatalf("scope=%s", crd.Spec.Scope)
	}
	if len(crd.Spec.Versions) != 1 || crd.Spec.Versions[0].Subresources == nil || crd.Spec.Versions[0].Subresources.Status == nil {
		t.Fatal("status subresource missing")
	}
}

// Kept behind this small seam so schema tests use the real API server.
func newDynamicClient(cfg *rest.Config) (dynamic.Interface, error) { return dynamic.NewForConfig(cfg) }
