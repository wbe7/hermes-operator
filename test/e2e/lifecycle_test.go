package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// One ordered suite keeps cluster mutations serial and evidence attributable.
func TestAcceptance(t *testing.T) {
	if os.Getenv("E2E_ENABLED") != "1" {
		t.Skip("run make test-e2e for the isolated kind suite")
	}
	// Every acceptance key is present even on an early failure. Partial local
	// success does not promote native or human scenarios to a passing release gate.
	type result struct {
		Status     string `json:"status"`
		LocalCheck string `json:"localCheck"`
	}
	results := map[string]result{}
	for i := 1; i <= 12; i++ {
		results[fmt.Sprintf("A%02d", i)] = result{"not-run", "not-run"}
	}
	defer func() {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		if err = os.WriteFile(filepath.Join(os.Getenv("E2E_DIR"), "acceptance.json"), data, 0600); err != nil {
			t.Error(err)
		}
	}()
	record := func(ids []string, ok bool) {
		for _, id := range ids {
			if ok {
				results[id] = result{"not-run", "passed (partial scope; see evidence ledger)"}
			} else {
				results[id] = result{"failed", "failed"}
			}
		}
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, script string, args ...string) {
		t.Helper()
		cmd := exec.Command("python3", append([]string{filepath.Join(root, "test/e2e/fixtures", script)}, args...)...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		t.Log(string(output))
		if err != nil {
			t.Fatal(err)
		}
	}
	if ok := t.Run("A09_NativeDualStackCNI", func(t *testing.T) { network(t, run) }); !ok {
		record([]string{"A09"}, false)
		return
	}
	record([]string{"A09"}, true)
	if !t.Run("A01_A08_A10_A12_ControllerLifecycle", func(t *testing.T) { run(t, "acceptance.py", "lifecycle") }) {
		record([]string{"A01", "A08", "A10", "A12"}, false)
		return
	}
	record([]string{"A01", "A08", "A10", "A12"}, true)
	ok := t.Run("A07_A12_VolumeRetentionUninstall", func(t *testing.T) { persistence(t, run) })
	record([]string{"A07", "A12"}, ok)
	if ok {
		results["A07"] = result{"passed", "passed"}
	}
	t.Run("A02_A03_Telegram", telegram)
}
