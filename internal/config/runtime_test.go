package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

// This test exercises Go output with the original image's native resolver, not a
// mocked parser. It never starts a gateway or makes a provider/Telegram request.
func TestOfficialRuntime(t *testing.T) {
	if os.Getenv("HERMES_RUNTIME_TEST") != "1" {
		t.Skip("set HERMES_RUNTIME_TEST=1 with the pinned official Docker image available")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err = os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, scenario := range []string{"default", "explicit", "noauth", "custom-host", "noauth-host", "retired"} {
		h, r := fixture()
		h.Spec.ExtraConfig = &runtime.RawExtension{Raw: []byte(`{"compression":{"threshold":0.7},"unknown_safe":{"nested":[1,2]}}`)}
		if scenario == "explicit" {
			h.Spec.Model.APIMode = "chat_completions"
			h.Spec.Reasoning.Overrides = map[string]string{"test-model": "high"}
			h.Spec.Telegram.Groups.Enabled = true
			h.Spec.Telegram.Groups.AllowedChatIDs = []string{"-456"}
		}
		if scenario == "noauth" || scenario == "noauth-host" {
			h.Spec.Model.Auth = "None"
		}
		if scenario == "custom-host" || scenario == "noauth-host" {
			h.Spec.Model.BaseURL = "https://openrouter.ai/api/v1"
		}
		if scenario == "retired" {
			h.Spec.ExtraConfig = nil
		}
		b, err := Render(h, r, sources())
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(directory, scenario, "input.json"), b.JSON)
		for name, data := range b.SecretData {
			write(filepath.Join(directory, scenario, "credentials", name), data)
		}
	}
	image := os.Getenv("HERMES_RUNTIME_IMAGE")
	if image == "" {
		image = "nousresearch/hermes-agent@sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294"
	}
	cmd := exec.Command("docker", "run", "--rm", "--pull=never", "--user", "10000:10000", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--network=none", "--tmpfs", "/tmp:rw,nosuid,nodev", "--entrypoint", "/opt/hermes/.venv/bin/python", "-v", root+"/runtime:/runtime:ro", "-v", root+"/internal/config/testdata:/checks:ro", "-v", directory+":/fixtures:ro", image, "-I", "/checks/verify_runtime.py")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("official runtime: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
