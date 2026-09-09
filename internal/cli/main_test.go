package cli

import (
	"fmt"
	"os"
	"testing"
)

// TestMain gives the package a garden of its own.
//
// This package's commands read and write the garden -- the pile, the draw
// log, the marks and now the event log's health -- and this is the package's
// first test, so the guard goes in before there is a second one that does not
// know it touches the garden. config.Sandbox turns forgetting into a panic
// rather than a silent write to the gardener's real garden, and this makes
// the panic unnecessary rather than merely loud.
func TestMain(m *testing.M) {
	garden, err := os.MkdirTemp("", "hugel-cli-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create test garden:", err)
		os.Exit(1)
	}
	os.Setenv("HUGEL_HOME", garden)
	code := m.Run()
	os.RemoveAll(garden)
	os.Exit(code)
}
