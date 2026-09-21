package e2e

import "testing"

func network(t *testing.T, run func(*testing.T, string, ...string)) { run(t, "network.py") }
