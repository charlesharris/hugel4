package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureStdout runs fn with os.Stdout swapped for a pipe and returns what it
// wrote, alongside fn's own error.
//
// The pipe is read to EOF in a goroutine, and the read result is received
// only after the writer is closed, so a report large enough to fill the pipe
// buffer cannot deadlock the test. Nothing in this file may call t.Parallel:
// this helper swaps a process-global.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w

	outCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outCh <- buf.String()
	}()

	fnErr := fn()

	w.Close()
	os.Stdout = orig
	out := <-outCh
	return out, fnErr
}

func TestHealthShowsNothingHasRunInAFreshGarden(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())

	out, err := captureStdout(t, func() error { return showHealth(false) })
	if err != nil {
		t.Fatalf("showHealth: %v", err)
	}
	if !strings.Contains(out, "healthy") {
		t.Errorf("output does not report the log as healthy:\n%s", out)
	}
	if !strings.Contains(out, "never") {
		t.Errorf("output does not say nothing has been recorded yet:\n%s", out)
	}
}

func TestHealthShowsWhenWritesHaveBeenFailing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HUGEL_HOME", dir)

	mark := filepath.Join(dir, "events.failing-since")
	if err := os.WriteFile(mark, nil, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	since := time.Date(2026, 9, 1, 9, 14, 0, 0, time.Local)
	if err := os.Chtimes(mark, since, since); err != nil {
		t.Fatalf("backdate marker: %v", err)
	}

	out, err := captureStdout(t, func() error { return showHealth(false) })
	if err != nil {
		t.Fatalf("showHealth: %v", err)
	}
	if !strings.Contains(out, "FAILING") {
		t.Errorf("output does not report FAILING:\n%s", out)
	}
	if !strings.Contains(out, "2026-09-01 09:14") {
		t.Errorf("output does not contain the formatted failure date:\n%s", out)
	}
}

func TestHealthSaysSoWhenTheGardenCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	notADir := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file standing in for the garden: %v", err)
	}
	t.Setenv("HUGEL_HOME", notADir)

	out, err := captureStdout(t, func() error { return showHealth(false) })
	if err != nil {
		t.Fatalf("showHealth returned an error instead of reporting the unreadable garden: %v", err)
	}
	if strings.Contains(out, "healthy") {
		t.Errorf("an unreadable garden must never render as healthy:\n%s", out)
	}
	if !strings.Contains(out, notADir) {
		t.Errorf("output does not name the unreadable path %q:\n%s", notADir, out)
	}
}

func TestHealthAsJSONCarriesBothFacts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HUGEL_HOME", dir)

	mark := filepath.Join(dir, "events.failing-since")
	if err := os.WriteFile(mark, nil, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	since := time.Date(2026, 9, 1, 9, 14, 0, 0, time.Local)
	if err := os.Chtimes(mark, since, since); err != nil {
		t.Fatalf("backdate marker: %v", err)
	}

	out, err := captureStdout(t, func() error { return showHealth(true) })
	if err != nil {
		t.Fatalf("showHealth: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal JSON output: %v\n%s", err, out)
	}
	if healthy, ok := got["healthy"].(bool); !ok || healthy {
		t.Errorf("expected healthy: false, got %v", got["healthy"])
	}
	if _, ok := got["failing_since"]; !ok {
		t.Errorf("expected a failing_since key in the JSON output, got %v", got)
	}
}
