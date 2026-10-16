// Package tier selects which tests run in a pass.
package tier

import (
	"os"
	"testing"
)

const (
	Unit        = "unit"
	Integration = "integration"
)

// Set skips t unless TEST_TIER is unset or equals tier.
func Set(t testing.TB, tier string) {
	t.Helper()
	run := os.Getenv("TEST_TIER")
	if run != "" && run != tier {
		t.Skipf("%s test, running %s tests", tier, run)
	}
}
