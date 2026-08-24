// Package draws records that soil was delivered.
//
// The pile's own files must not change when an entry is merely read — a draw
// that dirtied the repository would make every lookup a commit. So the record
// of what was drawn lives beside the pile rather than inside it, derived and
// disposable like any other index over it.
//
// This is the only instrument that can answer the two questions soil shipped
// without: whether an agent reaches for the pile at all, and whether what it
// hands back turns out to be worth keeping.
package draws

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charris/hugel/internal/config"
)

// Draw is one delivery of soil.
type Draw struct {
	At         time.Time `json:"at"`
	Bed        string    `json:"bed,omitempty"`
	Query      string    `json:"query"`
	Budget     int       `json:"budget"`
	Tokens     int       `json:"tokens"`
	Considered int       `json:"considered"`
	Entries    []string  `json:"entries,omitempty"`
}

// Path is where draws are recorded.
func Path() (string, error) {
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "draws.jsonl"), nil
}

// Append records a draw. Appending one line is atomic enough at this size, and
// the format is readable without a tool, which is the standard the pile is
// held to as well.
func Append(d Draw) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create garden dir: %w", err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal draw: %w", err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open draw log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write draw log: %w", err)
	}
	return nil
}

// Load reads every recorded draw, oldest first. A missing log is no draws, not
// an error: a garden that has never been asked anything is a normal garden.
func Load() ([]Draw, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read draw log: %w", err)
	}
	defer f.Close()

	var out []Draw
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var d Draw
		if err := json.Unmarshal(line, &d); err != nil {
			continue // a corrupt line should not cost the whole history
		}
		out = append(out, d)
	}
	return out, sc.Err()
}
