package pile

import (
	"fmt"
	"strings"
)

// Get returns the entry whose id begins with prefix. Ids are shown truncated
// everywhere a gardener actually meets one — in a draw, in a listing — so a
// prefix is the form that gets typed.
func (s *Store) Get(prefix string) (*Entry, error) {
	if err := s.load(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(prefix) == "" {
		return nil, fmt.Errorf("need an entry id")
	}
	var match *Entry
	for id, rel := range s.index {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		e, err := s.read(rel)
		if err != nil {
			return nil, err
		}
		if match != nil {
			return nil, fmt.Errorf("id prefix %q is ambiguous", prefix)
		}
		match = e
	}
	if match == nil {
		return nil, fmt.Errorf("no entry matching %q", prefix)
	}
	return match, nil
}

// Update writes a stored entry back where it already lives.
//
// It exists beside Put because the two are opposites. Put is convergence for
// extraction, and deliberately refuses to overwrite a human's standing on an
// entry; Update is that standing being set. Neither identity nor content hash
// is re-derived here: judging an entry wrong is not new knowledge about its
// subject, and an entry that moved file would lose its lineage in the log.
func (s *Store) Update(e *Entry) error {
	if err := s.load(); err != nil {
		return err
	}
	rel, ok := s.index[e.ID]
	if !ok {
		return fmt.Errorf("entry %s is not in the pile", e.ID)
	}
	if err := e.Validate(); err != nil {
		return fmt.Errorf("entry %q: %w", e.Title, err)
	}
	return s.write(rel, e)
}

// SetReview records what a human decided about an entry's trustworthiness.
// An entry already in that state is returned untouched, so re-reviewing what
// you reviewed last week leaves the repository clean.
func (s *Store) SetReview(prefix string, r Review) (*Entry, Result, error) {
	e, err := s.Get(prefix)
	if err != nil {
		return nil, "", err
	}
	if e.Review == r {
		return e, Unchanged, nil
	}
	e.Review = r
	if err := s.Update(e); err != nil {
		return nil, "", err
	}
	return e, Updated, nil
}

// SetStatus records what happened to the thing an entry describes, which is a
// different question from whether the entry is true. An abandoned approach was
// often correctly recorded; it is the approach that died, not the record.
func (s *Store) SetStatus(prefix string, st Status) (*Entry, Result, error) {
	e, err := s.Get(prefix)
	if err != nil {
		return nil, "", err
	}
	if e.Status == st {
		return e, Unchanged, nil
	}
	e.Status = st
	if err := s.Update(e); err != nil {
		return nil, "", err
	}
	return e, Updated, nil
}

// Supersede records that newer knowledge replaced older. The old entry sinks in
// soil rather than vanishing — it was true once, and why it stopped being true
// is worth reaching — and the link is written on the newer entry, pointing back,
// because that is the direction a reader travels.
func (s *Store) Supersede(oldPrefix, newPrefix string) (old, newer *Entry, err error) {
	if old, err = s.Get(oldPrefix); err != nil {
		return nil, nil, err
	}
	if newer, err = s.Get(newPrefix); err != nil {
		return nil, nil, err
	}
	if old.ID == newer.ID {
		return nil, nil, fmt.Errorf("entry %s cannot supersede itself", old.ID)
	}

	if !newer.linked("supersedes", old.ID) {
		newer.Links = append(newer.Links, Link{Rel: "supersedes", ID: old.ID})
		if err = s.Update(newer); err != nil {
			return nil, nil, err
		}
	}
	if old.Status != Superseded {
		old.Status = Superseded
		if err = s.Update(old); err != nil {
			return nil, nil, err
		}
	}
	return old, newer, nil
}

func (e *Entry) linked(rel, id string) bool {
	for _, l := range e.Links {
		if l.Rel == rel && l.ID == id {
			return true
		}
	}
	return false
}
