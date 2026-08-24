package tend

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charris/hugel/internal/pile"
)

// Store is what the surface needs to record a judgement. An interface rather
// than the concrete pile keeps the surface testable and keeps the write path
// exactly the one `hugel pile review` uses.
type Store interface {
	SetReview(id string, r pile.Review) (*pile.Entry, pile.Result, error)
	SetStatus(id string, s pile.Status) (*pile.Entry, pile.Result, error)
	Supersede(oldID, newID string) (*pile.Entry, *pile.Entry, error)
	Commit(message string) error
}

var (
	dim      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	head     = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	sel      = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	kept     = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	tossed   = lipgloss.NewStyle().Foreground(lipgloss.Color("167"))
	pending  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	titleBar = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
)

// Pane is one list the surface can show. Work and knowledge are two views of
// one sitting rather than two programs: what is in flight, and what the last
// flight left behind.
type Pane struct {
	Name string
	Rows []Row
}

// Model is the working surface.
type Model struct {
	act    Activity
	store  Store
	garden Garden
	panes  []Pane
	pane   int

	cursor  int
	offset  int // first visible row
	preview int // lines scrolled in the right pane
	width   int
	height  int

	// pendingSupersede holds the entry waiting for a replacement, so that
	// superseding needs two keystrokes and no typed id.
	pendingSupersede *pile.Entry

	cursors []int // where the gardener was in each pane
	judged  int
	status  string
	err     error
}

// New builds the surface over what the garden did, showing at most limit
// entries per group.
func New(a Activity, s Store, limit int) Model {
	return NewPanes([]Pane{{Name: "knowledge", Rows: a.Rows(limit)}}, a, s)
}

// NewGarden builds the surface over work first, with knowledge a keystroke
// away. The order is the entry point's argument: you arrive to see what is in
// flight, and judging what came in is what you do once you have looked.
func NewGarden(g Garden, a Activity, s Store, limit int) Model {
	m := NewPanes([]Pane{
		{Name: "work", Rows: g.Rows(limit)},
		{Name: "knowledge", Rows: a.Rows(limit)},
	}, a, s)
	m.garden = g
	return m
}

// NewPanes builds a surface over any set of panes.
func NewPanes(panes []Pane, a Activity, s Store) Model {
	m := Model{act: a, store: s, panes: panes, width: 100, height: 30}
	m.cursor = m.nextSelectable(-1, 1)
	m.cursors = make([]int, len(panes))
	m.cursors[0] = m.cursor
	return m
}

// paneRows finds a pane by name. The work header reports what is waiting on the
// knowledge side, and it must report the reachable number rather than the whole
// window: the cap exists so that the count can be got to in one sitting, and a
// header that ignores it undoes the discipline it is meant to show.
func (m Model) paneRows(name string) []Row {
	for _, p := range m.panes {
		if p.Name == name {
			return p.Rows
		}
	}
	return nil
}

// rows are the current pane's.
func (m Model) rows() []Row {
	if m.pane < 0 || m.pane >= len(m.panes) {
		return nil
	}
	return m.panes[m.pane].Rows
}

// switchPane keeps each pane's place. Losing your position in the work list
// because you glanced at knowledge is the kind of small rudeness that stops a
// surface being sat in front of.
func (m Model) switchPane(step int) Model {
	if len(m.panes) < 2 {
		return m
	}
	m.cursors[m.pane] = m.cursor
	m.pane = (m.pane + step + len(m.panes)) % len(m.panes)
	m.cursor = m.cursors[m.pane]
	if m.cursor == 0 || m.rows()[m.cursor].Kind == Heading {
		m.cursor = m.nextSelectable(-1, 1)
	}
	m.offset, m.preview, m.status = 0, 0, ""
	m.scroll()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

// nextSelectable walks past headings, which are labels rather than work.
func (m Model) nextSelectable(from, step int) int {
	rows := m.rows()
	for i := from + step; i >= 0 && i < len(rows); i += step {
		if rows[i].Kind != Heading {
			return i
		}
	}
	return from
}

// row is what the cursor is on, or nil on an empty pane.
func (m Model) row() *Row {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return nil
	}
	return &rows[m.cursor]
}

// current is the entry under the cursor, or nil when the cursor is on work.
// Verdicts belong to knowledge; a bead's standing is bd's business.
func (m Model) current() *pile.Entry {
	if r := m.row(); r != nil {
		return r.Entry
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, m.quit()
	case "j", "down":
		m.cursor, m.preview = m.nextSelectable(m.cursor, 1), 0
	case "k", "up":
		m.cursor, m.preview = m.nextSelectable(m.cursor, -1), 0
	case "g", "home":
		m.cursor, m.preview = m.nextSelectable(-1, 1), 0
	case "G", "end":
		m.cursor, m.preview = m.nextSelectable(len(m.rows()), -1), 0
	case "ctrl+d", "pgdown":
		m.preview += 10
	case "ctrl+u", "pgup":
		if m.preview -= 10; m.preview < 0 {
			m.preview = 0
		}
	case "a":
		m = m.judge("accept")
	case "r":
		m = m.judge("reject")
	case "x":
		m = m.judge("abandon")
	case "u":
		m = m.judge("unreview")
	case "s":
		m = m.supersede()
	case "tab", "right", "l":
		m = m.switchPane(1)
	case "shift+tab", "left", "h":
		m = m.switchPane(-1)
	}
	m.scroll()
	return m, nil
}

// judge records a verdict and moves on. Advancing after a keystroke is the
// whole ergonomic argument for a surface over a command line: the gardener
// holds the context for one entry, spends it, and gets the next.
func (m Model) judge(action string) Model {
	e := m.current()
	if e == nil {
		return m
	}
	var (
		got *pile.Entry
		res pile.Result
		err error
	)
	switch action {
	case "accept":
		got, res, err = m.store.SetReview(e.ID, pile.Accepted)
	case "reject":
		got, res, err = m.store.SetReview(e.ID, pile.Rejected)
	case "unreview":
		got, res, err = m.store.SetReview(e.ID, pile.Unreviewed)
	case "abandon":
		got, res, err = m.store.SetStatus(e.ID, pile.Abandoned)
	}
	if err != nil {
		m.err = err
		return m
	}
	*e = *got
	if res != pile.Unchanged {
		m.judged++
	}
	m.status = fmt.Sprintf("%sed · %s", action, truncate(e.Title, 44))
	m.cursor = m.nextSelectable(m.cursor, 1)
	m.preview = 0
	return m
}

// supersede takes two keystrokes on two entries rather than a typed id: the
// gardener already has both in front of them, which is the reason to be here.
func (m Model) supersede() Model {
	e := m.current()
	if e == nil {
		return m
	}
	if m.pendingSupersede == nil {
		m.pendingSupersede = e
		m.status = "superseded by? move to the newer entry and press s again (esc cancels)"
		return m
	}
	if m.pendingSupersede.ID == e.ID {
		m.pendingSupersede = nil
		m.status = "an entry cannot supersede itself"
		return m
	}
	old, _, err := m.store.Supersede(m.pendingSupersede.ID, e.ID)
	if err != nil {
		m.err, m.pendingSupersede = err, nil
		return m
	}
	*m.pendingSupersede = *old
	m.status = fmt.Sprintf("superseded · %s", truncate(old.Title, 40))
	m.pendingSupersede = nil
	m.judged++
	return m
}

// quit commits once rather than per keystroke. A sitting is one act of
// gardening, and its history reads better as one.
func (m Model) quit() tea.Cmd {
	if m.judged > 0 {
		if err := m.store.Commit(fmt.Sprintf("Tend: %d judgements", m.judged)); err != nil {
			m.err = err
		}
	}
	return tea.Quit
}

func (m *Model) scroll() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) listHeight() int {
	h := m.height - 6 // header, rule, status, keys
	if h < 3 {
		h = 3
	}
	return h
}

func (m Model) listWidth() int {
	w := m.width * 45 / 100
	if w < 28 {
		w = 28
	}
	if w > 60 {
		w = 60
	}
	return w
}

// View draws the surface: what happened on the left, the entry in full on the
// right. Judging without navigating away is the difference between a working
// surface and a dashboard.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	b.WriteString(dim.Render(strings.Repeat("─", m.width)) + "\n")

	lw, h := m.listWidth(), m.listHeight()
	list := m.listLines(lw, h)
	body := m.previewLines(m.width-lw-3, h)
	for i := 0; i < h; i++ {
		left, right := "", ""
		if i < len(list) {
			left = list[i]
		}
		if i < len(body) {
			right = body[i]
		}
		b.WriteString(fmt.Sprintf("%-*s %s %s\n",
			lw, truncateWide(left, lw), dim.Render("│"), right))
	}

	b.WriteString(dim.Render(strings.Repeat("─", m.width)) + "\n")
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) header() string {
	s := m.act.Soil
	name := "hugel tend"
	if len(m.panes) > 1 {
		name = "hugel garden"
	}
	left := titleBar.Render(name)

	var bits []string
	if m.panes[m.pane].Name == "work" {
		ready, active, blocked := m.garden.Totals()
		if active > 0 {
			bits = append(bits, fmt.Sprintf("%d in flight", active))
		}
		bits = append(bits, fmt.Sprintf("%d ready", ready))
		if blocked > 0 {
			bits = append(bits, fmt.Sprintf("%d blocked", blocked))
		}
		bits = append(bits, fmt.Sprintf("%d beds", len(m.garden.Beds)))
		if n := Unjudged(m.paneRows("knowledge")); n > 0 {
			bits = append(bits, fmt.Sprintf("%d to judge", n))
		}
		right := dim.Render(strings.Join(bits, " · "))
		gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}
		return left + strings.Repeat(" ", gap) + right
	}

	bits = []string{
		fmt.Sprintf("%d to judge", Unjudged(m.rows())),
		plural(s.Draws, "draw"),
	}
	if s.Sessions > 0 {
		bits = append(bits, fmt.Sprintf("reach %d/%d", s.Reached, s.Sessions))
	}
	if m.act.Barren > 0 {
		bits = append(bits, fmt.Sprintf("%d sessions composted to nothing", m.act.Barren))
	}
	if s.Judged() > 0 {
		bits = append(bits, fmt.Sprintf("precision %.0f%%", s.Precision()*100))
	}
	right := dim.Render(strings.Join(bits, " · "))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) listLines(w, h int) []string {
	out := make([]string, 0, h)
	rows := m.rows()
	for i := m.offset; i < len(rows) && len(out) < h; i++ {
		r := rows[i]
		if r.Kind == Heading {
			out = append(out, head.Render(r.Label))
			continue
		}
		mark := "  "
		if i == m.cursor {
			mark = "▸ "
		}
		var line string
		if r.Bead != nil {
			line = mark + workGlyph(r.Bead) + " " + truncate(r.Bead.Title, w-6)
		} else {
			if m.pendingSupersede != nil && m.pendingSupersede.ID == r.Entry.ID {
				mark = "↑ "
			}
			line = mark + standing(r.Entry) + " " + truncate(r.Entry.Title, w-6)
		}
		if i == m.cursor {
			line = sel.Render(line)
		}
		out = append(out, line)
	}
	return out
}

// standing is one character, because it appears on every row and the title is
// what the gardener is actually reading.
func standing(e *pile.Entry) string {
	switch {
	case e.Status == pile.Abandoned:
		return tossed.Render("×")
	case e.Status == pile.Superseded:
		return dim.Render("↓")
	case e.Review == pile.Accepted:
		return kept.Render("✓")
	case e.Review == pile.Rejected:
		return tossed.Render("✗")
	default:
		return pending.Render("·")
	}
}

func (m Model) previewLines(w, h int) []string {
	row := m.row()
	if row != nil && row.Bead != nil {
		return clamp(workDetail(row.Bead, row.Bed, w), m.preview, h)
	}
	e := m.current()
	if e == nil {
		if m.panes[m.pane].Name == "work" {
			return []string{dim.Render("no bed is tracking work.")}
		}
		return []string{dim.Render("nothing to judge in this window.")}
	}
	meta := fmt.Sprintf("%s · %s · %s", e.Type, e.Bed, e.Review)
	if e.Status != pile.Active {
		meta += " · " + string(e.Status)
	}
	lines := []string{dim.Render(meta), ""}
	for _, l := range wrap(e.Title, w) {
		lines = append(lines, titleBar.Render(l))
	}
	lines = append(lines, "")
	lines = append(lines, wrap(e.Body, w)...)

	return clamp(lines, m.preview, h)
}

// clamp scrolls a preview and cuts it to the pane.
func clamp(lines []string, from, h int) []string {
	if from >= len(lines) {
		from = len(lines) - 1
	}
	if from < 0 {
		from = 0
	}
	lines = lines[from:]
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

func (m Model) footer() string {
	if m.err != nil {
		return tossed.Render("error: " + m.err.Error())
	}
	keys := "j/k move · a keep · r toss · x abandon · s supersede · u undo · ^d/^u scroll · q done"
	if m.panes[m.pane].Name == "work" {
		keys = "j/k move · tab knowledge · ^d/^u scroll · q done"
	} else if len(m.panes) > 1 {
		keys = "j/k move · tab work · a keep · r toss · x abandon · s supersede · u undo · q done"
	}
	keys = dim.Render(keys)
	if m.status == "" {
		return keys
	}
	return m.status + "\n" + keys
}

// plural spares the surface from reporting "1 draws" at a gardener who is
// looking at exactly one draw.
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func wrap(s string, w int) []string {
	if w < 20 {
		w = 20
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) > w {
				out = append(out, line)
				line = word
				continue
			}
			line += " " + word
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if n < 4 {
		n = 4
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// truncateWide trims by display width, leaving escape sequences intact. Cutting
// styled text by byte offset splits an escape and bleeds colour down the pane.
func truncateWide(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, "…")
}
