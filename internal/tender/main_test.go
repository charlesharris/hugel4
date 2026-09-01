package tender

import (
	"fmt"
	"os"
	"testing"
)

// TestMain gives the package a garden of its own; see the same file in
// internal/gate for why the guard alone is not the whole fix.
//
// Start and Stop emit, so a test that only wanted to know whether a tender was
// running was writing to the gardener's log on the way past.
func TestMain(m *testing.M) {
	garden, err := os.MkdirTemp("", "hugel-tender-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create test garden:", err)
		os.Exit(1)
	}
	os.Setenv("HUGEL_HOME", garden)
	code := m.Run()
	os.RemoveAll(garden)
	os.Exit(code)
}
