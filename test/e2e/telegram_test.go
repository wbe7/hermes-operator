package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type telegramCase struct {
	Status string `json:"status"`
}
type telegramReport struct {
	Cases    map[string]telegramCase
	Evidence string
}

// A reused E2E_DIR can contain previous successes. Only an actually executed
// invocation's newly allocated directory can supply evidence to this run.
func invokeTelegram(send, config, dir string, execute func(string) ([]byte, error)) (*telegramReport, []byte, error) {
	if send != "yes" || config == "" {
		return nil, nil, nil
	}
	if dir == "" {
		return nil, nil, fmt.Errorf("E2E_DIR is required for invocation-scoped Telegram evidence")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	runDir, err := os.MkdirTemp(dir, "telegram-run-")
	if err != nil {
		return nil, nil, err
	}
	output, runErr := execute(runDir)
	if runErr != nil {
		exit, ok := runErr.(*exec.ExitError)
		if !ok || (exit.ExitCode() != 1 && exit.ExitCode() != 2) {
			return nil, output, runErr
		}
	}
	data, err := os.ReadFile(filepath.Join(runDir, "telegram-results.json"))
	if err != nil {
		return nil, output, fmt.Errorf("current Telegram invocation has no report: %w", err)
	}
	var cases map[string]telegramCase
	if err = json.Unmarshal(data, &cases); err != nil {
		return nil, output, err
	}
	evidence, err := filepath.Rel(dir, filepath.Join(runDir, "telegram-results.json"))
	if err != nil {
		return nil, output, err
	}
	return &telegramReport{Cases: cases, Evidence: evidence}, output, runErr
}

func telegram(t *testing.T, current **telegramReport) {
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
	report, output, err := invokeTelegram(os.Getenv("E2E_TELEGRAM_SEND"), os.Getenv("E2E_TELEGRAM_CONFIG"), os.Getenv("E2E_DIR"), func(runDir string) ([]byte, error) {
		cmd := exec.Command(python, script)
		cmd.Env = append(os.Environ(), "E2E_DIR="+runDir)
		return cmd.CombinedOutput()
	})
	*current = report
	t.Log(string(output))
	if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 2 {
		t.Skip("required live cases lack usable evidence; see the current invocation report")
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
	var current *telegramReport
	telegram(t, &current)
}
