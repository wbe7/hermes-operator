package e2e

import "testing"

func persistence(t *testing.T, run func(*testing.T, string, ...string)) {
	run(t, "acceptance.py", "persistence")
}
