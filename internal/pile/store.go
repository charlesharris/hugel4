package pile

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charris/hugel/internal/config"
)

// DefaultRoot is where the pile lives unless told otherwise.
//
// It hangs off the garden rather than off the user's home directory, so moving
// the garden moves the pile with it. Reaching for the home directory directly
// meant HUGEL_HOME relocated the config, the draw log and the marks but left
// the pile behind -- which is fine until a test writes to the real one.
func DefaultRoot() (string, error) {
	// HUGEL_PILE is the one path that does not come through config.Home, so it
	// is the one path the sandbox has to be asked for by name.
	if r := os.Getenv("HUGEL_PILE"); r != "" {
		return config.Sandbox(r), nil
	}
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "pile"), nil
}

// Store is a pile on disk: a directory of JSON entries under git.
//
// Git is the storage mechanism, not something the gardener operates. hugel
// initialises the repository, commits what it writes, and can report when the
// pile has drifted from its remote. It also supplies the temporal layer for
// free: superseding an entry is a commit, and an entry's lineage is its log.
type Store struct {
	Root string

	index map[string]string // entry id -> path relative to Root
}

// Result says what a write actually did.
type Result string

const (
	Created   Result = "created"
	Updated   Result = "updated"
	Unchanged Result = "unchanged"
)

// ErrNoPile is returned when a directory is not an initialised pile.
var ErrNoPile = errors.New("not a pile: run hugel pile init")

// Open loads an existing pile.
func Open(root string) (*Store, error) {
	if _, err := os.Stat(filepath.Join(root, "entries")); err != nil {
		return nil, fmt.Errorf("%s: %w", root, ErrNoPile)
	}
	return &Store{Root: root}, nil
}

// Init creates a pile, including its git repository. It is safe to run on an
// existing pile.
func Init(root string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, "entries"), 0o755); err != nil {
		return nil, fmt.Errorf("create pile: %w", err)
	}
	readme := filepath.Join(root, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		if err := os.WriteFile(readme, []byte(pileReadme), 0o644); err != nil {
			return nil, fmt.Errorf("write pile readme: %w", err)
		}
	}
	s := &Store{Root: root}
	if !s.isRepo() {
		if err := s.git("init", "-q"); err != nil {
			return nil, fmt.Errorf("init pile repository: %w", err)
		}
	}
	return s, nil
}

const pileReadme = `# pile

This is a hugel pile: composted knowledge from development sessions, shared
across every bed.

The files here are the source of truth. Any index built over them -- search,
embeddings, graph -- is derived and disposable, rebuildable by re-reading this
directory. An earlier Hugel made a graph database authoritative, which meant the
knowledge died with a container volume.

Entries are JSON, one per file, grouped by bed. ` + "`entries/general/`" + ` holds
knowledge that is not specific to any project and travels to all of them.

Written and committed by hugel. It is private: entries are composted from
session transcripts, so treat this repository the way you would treat those.
`

// Put writes an entry, converging rather than duplicating: an entry whose
// identity already exists is updated in place, and one whose content is
// unchanged is not rewritten at all, so re-composting a session leaves the
// repository clean.
func (s *Store) Put(e *Entry) (Result, error) {
	e.Seal(time.Now())
	if err := e.Validate(); err != nil {
		return "", fmt.Errorf("entry %q: %w", e.Title, err)
	}
	if err := s.load(); err != nil {
		return "", err
	}

	path := e.Filename()
	if old, ok := s.index[e.ID]; ok {
		existing, err := s.read(old)
		if err != nil {
			return "", err
		}
		if existing.ContentHash == e.ContentHash && existing.Status == e.Status && existing.Review == e.Review {
			return Unchanged, nil
		}
		// Preserve the standing a human gave it; extraction does not get to
		// un-review or resurrect an entry someone abandoned.
		e.Review = existing.Review
		if existing.Status != Active {
			e.Status = existing.Status
		}
		e.CreatedAt = existing.CreatedAt
		e.Seal(time.Now())
		if old != path {
			if err := os.Remove(filepath.Join(s.Root, old)); err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("remove renamed entry: %w", err)
			}
		}
		if err := s.write(path, e); err != nil {
			return "", err
		}
		s.index[e.ID] = path
		return Updated, nil
	}

	// Two different entries can want the same filename on the same day. Give
	// the loser a short discriminator rather than silently overwriting.
	path = s.freeName(path, e.ID)
	if err := s.write(path, e); err != nil {
		return "", err
	}
	s.index[e.ID] = path
	return Created, nil
}

// Has reports whether an entry is in the pile. It is the question a link has to
// be able to ask before it is written, since an edge to nothing reads as
// evidence until someone tries to follow it.
func (s *Store) Has(id string) bool {
	if err := s.load(); err != nil {
		return false
	}
	_, ok := s.index[id]
	return ok
}

// All returns every entry, newest occurrence first.
func (s *Store) All() ([]*Entry, error) {
	if err := s.load(); err != nil {
		return nil, err
	}
	out := make([]*Entry, 0, len(s.index))
	for _, p := range s.index {
		e, err := s.read(p)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].OccurredAt.After(out[j].OccurredAt)
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}

// Count reports how many entries the pile holds.
func (s *Store) Count() (int, error) {
	if err := s.load(); err != nil {
		return 0, err
	}
	return len(s.index), nil
}

func (s *Store) load() error {
	if s.index != nil {
		return nil
	}
	s.index = map[string]string{}
	root := filepath.Join(s.Root, "entries")
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(s.Root, p)
		if err != nil {
			return err
		}
		e, err := s.read(rel)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		s.index[e.ID] = rel
		return nil
	})
}

func (s *Store) read(rel string) (*Entry, error) {
	b, err := os.ReadFile(filepath.Join(s.Root, rel))
	if err != nil {
		return nil, fmt.Errorf("read entry: %w", err)
	}
	return Unmarshal(b)
}

func (s *Store) write(rel string, e *Entry) error {
	full := filepath.Join(s.Root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create entry dir: %w", err)
	}
	b, err := e.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(full, b, 0o644); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return nil
}

// freeName returns a path not already taken by a different entry.
func (s *Store) freeName(path, id string) string {
	taken := map[string]bool{}
	for _, p := range s.index {
		taken[p] = true
	}
	if !taken[path] {
		return path
	}
	base := strings.TrimSuffix(path, ".json")
	return fmt.Sprintf("%s-%s.json", base, id[:6])
}

// --- git ---------------------------------------------------------------

func (s *Store) isRepo() bool {
	_, err := os.Stat(filepath.Join(s.Root, ".git"))
	return err == nil
}

func (s *Store) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Commit records the current state of the pile. A pile with nothing to commit
// is not an error: hugel commits after every write, and most writes change
// nothing.
func (s *Store) Commit(message string) error {
	if !s.isRepo() {
		return nil
	}
	if err := s.git("add", "-A"); err != nil {
		return err
	}
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = s.Root
	if err := cmd.Run(); err == nil {
		return nil // nothing staged
	}
	return s.git("commit", "-q", "-m", message)
}
