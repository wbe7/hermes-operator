package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func telegram(t *testing.T) {
	if os.Getenv("E2E_TELEGRAM_SEND") != "yes" || os.Getenv("E2E_TELEGRAM_CONFIG") == "" {
		t.Skip("A02/A03 not-run: explicit send opt-in and dedicated client configuration required; see test-e2e-telegram")
	}
	script, err := filepath.Abs("../../hack/e2e-telegram.py")
	if err != nil {
		t.Fatal(err)
	}
	python := os.Getenv("E2E_TELEGRAM_PYTHON")
	if python == "" {
		python = "python3"
	}
	output, err := exec.Command(python, script).CombinedOutput()
	t.Log(string(output))
	if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 2 {
		t.Skip("required live cases lack usable evidence; see telegram-results.json")
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Can run independently of the disposable local cluster.
func TestTelegramLive(t *testing.T) {
	if os.Getenv("E2E_ENABLED") == "1" {
		t.Skip("the local acceptance suite already invokes its Telegram subtest")
	}
	telegram(t)
}
