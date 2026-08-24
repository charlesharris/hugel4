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
func (g Garden) Totals() (ready, active, blocked int) {
	for _, w := range g.Beds {
		r, a, b := w.Counts()
		ready, active, blocked = ready+r, active+a, blocked+b
	}
	return ready, active, blocked
}

// Rows lists each bed and at most limit of its beads.
//
// In flight first, then ready, then blocked. That order is the question a
// gardener is actually asking: what is half-finished and waiting on me, what
// could be started, and what cannot move. The cap is the same refusal the
// knowledge side makes — one bed with thirty ready beads must not bury the
// other beds, and what is left out is stated.
func (g Garden) Rows(limit int) []Row {
	var rows []Row
	for _, w := range g.Beds {
		ready, active, blocked := w.Counts()
		shown := w.Beads
		sort.SliceStable(shown, func(i, j int) bool {
			return workRank(shown[i]) < workRank(shown[j])
		})
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
		}
		rows = append(rows, Row{
			Kind:  Heading,
			Label: bedHeading(w.Bed, ready, active, blocked, len(shown), len(w.Beads)),
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
	case b.Status == "in_progress":
		return 0
	case b.Ready:
		return 1
	default:
		return 2
	}
}

func bedHeading(bed string, ready, active, blocked, shown, total int) string {
	var parts []string
	if active > 0 {
		parts = append(parts, fmt.Sprintf("%d in flight", active))
	}
	parts = append(parts, fmt.Sprintf("%d ready", ready))
	if blocked > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", blocked))
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
	if strings.TrimSpace(b.Body) != "" {
		lines = append(lines, "")
		lines = append(lines, wrap(b.Body, w)...)
	}
	return lines
}
