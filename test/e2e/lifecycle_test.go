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
		Status     string   `json:"status"`
		LocalCheck string   `json:"localCheck"`
		Scope      string   `json:"scope"`
		Evidence   []string `json:"evidence"`
	}
	results := map[string]result{}
	for i := 1; i <= 12; i++ {
		results[fmt.Sprintf("A%02d", i)] = result{Status: "not-run", LocalCheck: "not-run", Scope: "not exercised by local suite", Evidence: []string{"tests.txt"}}
	}
	defer func() {
		scopes := map[string]string{
			"A01": "two namespaces; distinct actual Pending Pod/PVC identities",
			"A02": "dedicated live Telegram DM, first contact and personalization; not a local fixture claim",
			"A03": "dedicated unauthorized sender/group transport and native non-dispatch proof",
			"A04": "native same-Pod session/config restoration not exercised locally",
			"A05": "native personalization/history replacement not exercised locally",
			"A06": "native extra-config removal not exercised locally",
			"A07": "actual PVC Retain/Delete/existingClaim and marker persistence",
			"A08": "local missing/deleted Secret and unsupported version only",
			"A09": "native dual-stack CNI and owned-node metadata DNAT stand-in; external IPv6 baseline limitation",
			"A10": "Pending workload suspend/resume and manager restart",
			"A11": "WaitForFirstConsumer; CSI expansion not exercised locally",
			"A12": "real Helm install/upgrade/uninstall; active Pending CR UID retention",
		}
		for id, row := range results {
			row.Scope = scopes[id]
			if row.LocalCheck != "not-run" {
				switch id {
				case "A09":
					row.Evidence = append(row.Evidence, "baseline.json", "isolated.json", "exception.json", "restored.json")
				case "A01", "A08", "A10":
					row.Evidence = append(row.Evidence, "lifecycle.json")
				case "A12":
					row.Evidence = append(row.Evidence, "lifecycle.json", "storage.json")
				}
			}
			existing := []string{}
			for _, path := range row.Evidence {
				if _, err := os.Stat(filepath.Join(os.Getenv("E2E_DIR"), path)); err == nil || path == "tests.txt" {
					existing = append(existing, path)
				}
			}
			row.Evidence = existing
			results[id] = row
		}
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
				results[id] = result{Status: "not-run", LocalCheck: "passed", Scope: "partial local fixture scope; native/human gates remain separate", Evidence: []string{"tests.txt"}}
			} else {
				results[id] = result{Status: "failed", LocalCheck: "failed", Scope: "local fixture failed", Evidence: []string{"tests.txt"}}
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
		results["A07"] = result{Status: "passed", LocalCheck: "passed", Scope: "Retain/Delete/existingClaim and marker on actual PVC", Evidence: []string{"tests.txt", "storage.json"}}
	}
	var current *telegramReport
	t.Run("A02_A03_Telegram", func(t *testing.T) { telegram(t, &current) })
	if current != nil {
		for id, names := range map[string][]string{"A02": {"allowed_dm", "first_contact", "personalization"}, "A03": {"unauthorized_dm", "denied_group_text", "denied_group_command", "denied_group_media"}} {
			status := "passed"
			for _, name := range names {
				if current.Cases[name].Status == "failed" {
					status = "failed"
					break
				}
				if current.Cases[name].Status != "passed" {
					status = "not-run"
				}
			}
			results[id] = result{Status: status, LocalCheck: "separate opt-in live clients", Scope: "dedicated Telegram transport + native SQLite/model/personalization evidence", Evidence: []string{current.Evidence}}
		}
	}
}
