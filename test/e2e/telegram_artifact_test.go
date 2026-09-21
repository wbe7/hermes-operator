package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const passedTelegramFixture = `{"allowed_dm":{"status":"passed"},"first_contact":{"status":"passed"},"personalization":{"status":"passed"},"unauthorized_dm":{"status":"passed"},"denied_group_text":{"status":"passed"},"denied_group_command":{"status":"passed"},"denied_group_media":{"status":"passed"}}`

func staleTelegramFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "telegram-results.json"), []byte(passedTelegramFixture), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestTelegramArtifactMissingConfigIgnoresStalePass(t *testing.T) {
	dir := staleTelegramFixture(t)
	report, _, err := invokeTelegram("yes", "", dir, func(string) ([]byte, error) { t.Fatal("must not execute without configuration"); return nil, nil })
	if err != nil || report != nil {
		t.Fatalf("missing configuration accepted artifact: report=%v err=%v", report, err)
	}
}

func TestTelegramArtifactFailedInvocationIgnoresStalePass(t *testing.T) {
	dir := staleTelegramFixture(t)
	report, _, err := invokeTelegram("yes", "dedicated-config", dir, func(runDir string) ([]byte, error) {
		if runDir == dir {
			t.Fatal("invocation must get a unique output directory")
		}
		return exec.Command("sh", "-c", "exit 1").CombinedOutput()
	})
	if err == nil || report != nil {
		t.Fatalf("failed invocation accepted stale artifact: report=%v err=%v", report, err)
	}
}

func TestTelegramArtifactCurrentInvocationAccepted(t *testing.T) {
	dir := staleTelegramFixture(t)
	report, _, err := invokeTelegram("yes", "dedicated-config", dir, func(runDir string) ([]byte, error) {
		return nil, os.WriteFile(filepath.Join(runDir, "telegram-results.json"), []byte(passedTelegramFixture), 0600)
	})
	if err != nil || report == nil {
		t.Fatalf("current report missing: %v", err)
	}
	if report.Cases["allowed_dm"].Status != "passed" {
		t.Fatal("current case not loaded")
	}
	if !strings.HasPrefix(report.Evidence, "telegram-run-") || report.Evidence == "telegram-results.json" {
		t.Fatalf("evidence is not invocation-scoped: %s", report.Evidence)
	}
	if _, err := os.Stat(filepath.Join(dir, report.Evidence)); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramArtifactSuccessfulExitWithoutReportIgnoresStalePass(t *testing.T) {
	dir := staleTelegramFixture(t)
	report, _, err := invokeTelegram("yes", "dedicated-config", dir, func(string) ([]byte, error) { return nil, nil })
	if err == nil || report != nil {
		t.Fatalf("missing current artifact accepted stale report: report=%v err=%v", report, err)
	}
}
