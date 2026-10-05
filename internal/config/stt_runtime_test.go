package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestSTTOfficialRuntime(t *testing.T) {
	if os.Getenv("HERMES_RUNTIME_TEST") != "1" {
		t.Skip("requires pinned Hermes image")
	}
	root, _ := filepath.Abs("../..")
	dir := t.TempDir()
	_ = os.Chmod(dir, 0755)
	for name, raw := range map[string]string{
		"inherited": `{"enabled":true,"model":"qwen3-asr-1.7b"}`,
		"override":  `{"enabled":true,"model":"asr-custom","baseURL":"http://127.0.0.1:18961/speech/v1","apiKeySecretRef":{},"language":"en","echoTranscripts":false}`,
		"auto":      `{"enabled":true,"model":"qwen3-asr-1.7b","language":"auto"}`,
		"none":      `{"enabled":true,"model":"qwen3-asr-1.7b","auth":"None"}`,
		"disabled":  `null`,
	} {
		h, r := fixture()
		h.Spec.Model.BaseURL = "http://127.0.0.1:18961/v1"
		withSTT(t, h, raw)
		src := sources()
		src[types.NamespacedName{Namespace: "tenant", Name: "maria-hermes-secret"}].Data["STT_API_KEY"] = []byte("separate-stt-key")
		b, err := Render(h, r, src)
		if err != nil {
			t.Fatal(err)
		}
		creds := map[string]string{}
		for k, v := range b.SecretData {
			creds[k] = string(v)
		}
		data, _ := json.Marshal(map[string]any{"bundle": json.RawMessage(b.JSON), "credentials": creds})
		if err = os.WriteFile(filepath.Join(dir, name+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	image := os.Getenv("HERMES_RUNTIME_IMAGE")
	if image == "" {
		image = "nousresearch/hermes-agent@sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294"
	}
	cmd := exec.Command("docker", "run", "--rm", "--pull=never", "--user", "10000:10000", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--network=none", "--tmpfs", "/tmp:rw,nosuid,nodev", "--entrypoint", "/opt/hermes/.venv/bin/python", "-v", root+"/runtime:/runtime:ro", "-v", root+"/internal/config/testdata:/checks:ro", "-v", dir+":/fixtures:ro", image, "-I", "/checks/verify_stt.py")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native STT: %v\n%s", err, out)
	}
	t.Log(string(out))
}
