// Package tend is the working surface: what the garden did lately, and the
// judgement a gardener passes on it.
//
// It is bounded by time, never by backlog. There is deliberately no view of
// every unreviewed entry — a queue of hundreds costs more to work than the pile
// saves, which is the first thing this garden refuses to build. Sit down after
// three days away and you are shown three days, not three hundred entries.
package tend

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/yield"
)

// Activity is what happened in the window.
type Activity struct {
	Since     time.Time
	Soil      yield.SoilReport
	Delivered []*pile.Entry // drawn from the pile, most recently drawn first
	Fresh     []*pile.Entry // composted into the pile, newest first
	Producing int           // sessions that yielded at least one entry
	Barren    int           // sessions that yielded nothing
}

// Kind distinguishes why a row is on the surface.
type Kind int

const (
	// Heading labels a group and cannot be selected.
	Heading Kind = iota
	// Drawn is an entry the pile handed to a session. These are judged first:
	// they are the only entries that have cost tokens rather than disk.
	Drawn
	// Composted is an entry composted in the window.
	Composted
)

// Row is one line of the list.
type Row struct {
	Kind  Kind
	Label string
	Entry *pile.Entry
}

// Gather assembles the surface. It does no IO, so what is shown can be tested
// without a terminal or a pile.
//
// home names the bed the gardener is standing in, and every earlier name for
// the same project. It orders rather than filters: with a cap on each group,
// what is shown first is what gets judged, and the project in front of you is
// the one you can judge. Nothing is hidden -- another bed's entries still
// appear below, because the pile is shared and they still need a verdict.
func Gather(entries []*pile.Entry, log []draws.Draw, soil yield.SoilReport, since time.Time, home []string) Activity {
	local := map[string]bool{}
	for _, n := range home {
		local[strings.ToLower(n)] = true
	}
	isLocal := func(e *pile.Entry) bool { return local[strings.ToLower(e.Bed)] }

	a := Activity{Since: since, Soil: soil}

	byID := map[string]*pile.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}

	// Most recently drawn first, and an entry drawn twice is listed once: the
	// question a gardener answers about it is the same either way.
	lastDrawn := map[string]time.Time{}
	for _, d := range log {
		if d.At.Before(since) {
			continue
		}
		for _, id := range d.Entries {
			if t, ok := lastDrawn[id]; !ok || d.At.After(t) {
				lastDrawn[id] = d.At
			}
		}
	}
	for id := range lastDrawn {
		if e := byID[id]; e != nil {
			a.Delivered = append(a.Delivered, e)
		}
	}
	sort.Slice(a.Delivered, func(i, j int) bool {
		return lastDrawn[a.Delivered[i].ID].After(lastDrawn[a.Delivered[j].ID])
	})

	sessions := map[string]bool{}
	for _, e := range entries {
		if e.CreatedAt.Before(since) {
			continue
		}
		if lastDrawn[e.ID].IsZero() { // a drawn entry is already on the surface
			a.Fresh = append(a.Fresh, e)
		}
		if s := e.Source.Session; s != "" {
			sessions[s] = true
		}
	}
	sort.Slice(a.Fresh, func(i, j int) bool {
		if li, lj := isLocal(a.Fresh[i]), isLocal(a.Fresh[j]); li != lj {
			return li
		}
		if !a.Fresh[i].CreatedAt.Equal(a.Fresh[j].CreatedAt) {
			return a.Fresh[i].CreatedAt.After(a.Fresh[j].CreatedAt)
		}
		return a.Fresh[i].Title < a.Fresh[j].Title
	})

	a.Producing = len(sessions)
	if a.Barren = soil.Sessions - a.Producing; a.Barren < 0 {
		a.Barren = 0
	}
	return a
}

// Rows renders the surface as a list, at most limit entries per group.
//
// The cap is what keeps a working surface from turning into the inbox this
// refuses to be. A bulk import or a first compost run drops hundreds of entries
// inside any window, and a list of hundreds is not judged, it is abandoned.
// What is left out is stated rather than silently dropped: a truncated list
// that looks complete is worse than a long one.
//
// Headings stay in place when a group is empty. An empty DELIVERED group means
// the pile was never asked, which is a finding, not a blank to be hidden.
func (a Activity) Rows(limit int) []Row {
	rows := []Row{{Kind: Heading, Label: heading("DELIVERED", len(a.Delivered), limit, "drawn from the pile")}}
	for i, e := range a.Delivered {
		if limit > 0 && i >= limit {
			break
		}
		rows = append(rows, Row{Kind: Drawn, Entry: e})
	}
	rows = append(rows, Row{Kind: Heading, Label: heading("NEW", len(a.Fresh), limit, "composted in")})
	for i, e := range a.Fresh {
		if limit > 0 && i >= limit {
			break
		}
		rows = append(rows, Row{Kind: Composted, Entry: e})
	}
	return rows
}

func heading(name string, n, limit int, what string) string {
	switch {
	case n == 0:
		return name + " · nothing " + what
	case limit > 0 && n > limit:
		return fmt.Sprintf("%s · %d of %d shown", name, limit, n)
	}
	return name
}

// Unjudged counts what is on the surface right now and still has no standing.
// It counts the rows shown rather than everything in the window, because it is
// the only progress number here and it has to be reachable in one sitting.
func Unjudged(rows []Row) int {
	n := 0
	for _, r := range rows {
		if r.Kind == Heading {
			continue
		}
		if r.Entry.Review == pile.Unreviewed && r.Entry.Status == pile.Active {
			n++
		}
	}
	return n
}
