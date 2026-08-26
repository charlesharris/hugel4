package tend

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charris/hugel/internal/beads"
)

// Garden is the work going out, as the surface shows it: every bed's open
// beads, in the order a gardener meets them.
//
// tend judges knowledge that came in; garden shows work going out. They are the
// same surface because they are the same sitting — you look at what is in
// flight, and you judge what the last flight left behind.
type Garden struct {
	Beds []*beads.Work
}

// Totals adds up every bed.
func (g Garden) Totals() beads.Tally {
	var t beads.Tally
	for _, w := range g.Beds {
		c := w.Counts()
		t.Attention += c.Attention
		t.Active += c.Active
		t.Ready += c.Ready
		t.Blocked += c.Blocked
	}
	return t
}

// Rows lists each bed and at most limit of its beads.
//
// What you owe first, then what is running, then what could start, then what
// cannot move. That order is the question a gardener sits down asking, and the
// answer that matters most is the one nothing else in the system will act on:
// a tender will pick up ready work unattended, and will never pick up work
// waiting on a person. The cap is the same refusal the
// knowledge side makes — one bed with thirty ready beads must not bury the
// other beds, and what is left out is stated.
func (g Garden) Rows(limit int) []Row {
	var rows []Row
	for _, w := range g.Beds {
		tally := w.Counts()
		shown := w.Beads
		sort.SliceStable(shown, func(i, j int) bool {
			return workRank(shown[i]) < workRank(shown[j])
		})
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
		}
		rows = append(rows, Row{
			Kind:  Heading,
			Label: bedHeading(w.Bed, tally, len(shown), len(w.Beads)),
			Bed:   w.Bed,
		})
		for i := range shown {
			rows = append(rows, Row{Kind: Work, Bead: &shown[i], Bed: w.Bed})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, Row{Kind: Heading, Label: "WORK · no bed is tracking any"})
	}
	return rows
}

// workRank orders a bed's beads by what they need from a gardener.
func workRank(b beads.Bead) int {
	switch {
	case b.Labeled(beads.NeedsAttention):
		return 0
	case b.Status == "in_progress":
		return 1
	case b.Ready:
		return 2
	default:
		return 3
	}
}

func bedHeading(bed string, t beads.Tally, shown, total int) string {
	var parts []string
	if t.Attention > 0 {
		parts = append(parts, fmt.Sprintf("%d need you", t.Attention))
	}
	if t.Active > 0 {
		parts = append(parts, fmt.Sprintf("%d in flight", t.Active))
	}
	parts = append(parts, fmt.Sprintf("%d ready", t.Ready))
	if t.Blocked > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", t.Blocked))
	}
	if shown < total {
		parts = append(parts, fmt.Sprintf("%d of %d shown", shown, total))
	}
	return fmt.Sprintf("%s · %s", strings.ToUpper(bed), strings.Join(parts, " · "))
}

// workGlyph is one character, for the same reason a standing is: the title is
// what a gardener is reading.
func workGlyph(b *beads.Bead) string {
	switch {
	case b.Labeled(beads.NeedsAttention):
		return tossed.Render("!")
	case b.Status == "in_progress":
		return pending.Render("◐")
	case b.Ready:
		return kept.Render("○")
	default:
		return dim.Render("●")
	}
}

func workDetail(b *beads.Bead, bed string, w int) []string {
	meta := fmt.Sprintf("%s · %s · P%d · %s", b.ID, b.Type, b.Priority, bed)
	switch {
	case b.Labeled(beads.NeedsAttention):
		meta += " · needs you"
	case b.Status == "in_progress":
		meta += " · in flight"
	case b.Ready:
		meta += " · ready"
	default:
		meta += " · blocked"
	}
	lines := []string{dim.Render(meta), ""}
	for _, l := range wrap(b.Title, w) {
		lines = append(lines, titleBar.Render(l))
	}
	// Why it stopped comes before what it is. A bead waiting on a person is the
	// one row where the reason is the whole point of looking at it, and the
	// description is what you already knew when you filed it.
	if b.Labeled(beads.NeedsAttention) && strings.TrimSpace(b.Notes) != "" {
		lines = append(lines, "")
		lines = append(lines, tossed.Render("why it stopped"))
		lines = append(lines, wrap(strings.TrimSpace(b.Notes), w)...)
	}
	if strings.TrimSpace(b.Body) != "" {
		lines = append(lines, "")
		lines = append(lines, wrap(b.Body, w)...)
	}
	return lines
}
