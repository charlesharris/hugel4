package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DefaultRoot is where Claude Code keeps session transcripts.
func DefaultRoot() (string, error) {
	if r := os.Getenv("HUGEL_TRANSCRIPT_ROOT"); r != "" {
		return r, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Discover returns every transcript file under root, newest last.
func Discover(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read transcript root %s: %w", root, err)
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue // an unreadable project dir should not sink the whole scan
		}
		for _, f := range files {
			if !f.IsDir() && filepath.Ext(f.Name()) == ".jsonl" {
				paths = append(paths, filepath.Join(dir, f.Name()))
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// LoadAll parses every transcript under root. Files that fail to parse are
// reported but do not abort the scan.
func LoadAll(root string) ([]*Session, []error, error) {
	paths, err := Discover(root)
	if err != nil {
		return nil, nil, err
	}
	var (
		sessions []*Session
		problems []error
	)
	for _, p := range paths {
		s, err := ParseFile(p)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if len(s.Requests) == 0 {
			continue // nothing was spent here
		}
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Start.Before(sessions[j].Start) })
	return sessions, problems, nil
}
