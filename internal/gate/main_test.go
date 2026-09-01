package gate

import (
	"fmt"
	"os"
	"testing"
)

// TestMain gives the package a garden of its own.
//
// The gate emits an event at every stage, so every test that runs a gate is an
// emitter whether it meant to be or not — which is how the gardener's real
// events log came to hold 471 events from a bead called x-1 and nothing else.
// Setting the garden once for the package is the fix that survives the next
// test being added, because the next test does not have to know it emits.
//
// config.Sandbox is what makes forgetting this loud rather than silent: without
// it the package still runs and still writes, just somewhere it should not.
func TestMain(m *testing.M) {
	garden, err := os.MkdirTemp("", "hugel-gate-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create test garden:", err)
		os.Exit(1)
	}
	os.Setenv("HUGEL_HOME", garden)
	code := m.Run()
	os.RemoveAll(garden)
	os.Exit(code)
}
