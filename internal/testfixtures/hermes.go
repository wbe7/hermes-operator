package testfixtures

import (
	_ "embed"
	"testing"

	v1alpha1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

//go:embed minimal.yaml
var minimal []byte

func Hermes(t testing.TB) *v1alpha1.Hermes {
	t.Helper()
	var h v1alpha1.Hermes
	if err := yaml.Unmarshal(minimal, &h); err != nil {
		t.Fatalf("unmarshal minimal Hermes fixture: %v", err)
	}
	h.UID = types.UID("fixture-hermes-uid")
	return &h
}
