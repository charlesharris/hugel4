package soil

import (
	"fmt"
	"strings"
)

// Item is one piece of soil as delivered: enough to judge whether the entry is
// worth reading in full, and a handle for doing so.
type Item struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Scope   string `json:"scope"`
	Bed     string `json:"bed"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	When    string `json:"when"`
	Review  string `json:"review,omitempty"`
	Partial bool   `json:"partial,omitempty"`
}

// Soil is what the pile offered for one piece of work, under budget.
type Soil struct {
	Query      string `json:"query"`
	Bed        string `json:"bed,omitempty"`
	Items      []Item `json:"items"`
	Tokens     int    `json:"tokens"`
	Considered int    `json:"considered"`
	Omitted    int    `json:"omitted,omitempty"`
}

// tokens estimates cost. Characters over four is close enough for budgeting and
// costs nothing to compute; being exactly right would mean asking a tokeniser,
// which would make drawing soil more expensive than the soil saves.
func tokens(s string) int { return (len(s) + 3) / 4 }

// Draw runs a query and packs the results into a token budget.
//
// The budget is the feature. An unbounded pile lookup that returns everything
// relevant would reproduce the problem soil exists to solve: context that
// arrives early, stays for the whole session, and is re-read on every turn.
func (ix *Index) Draw(q Query) *Soil {
	if q.Budget <= 0 {
		q.Budget = defaultBudget
	}
	if q.Snippet <= 0 {
		q.Snippet = defaultSnippet
	}
	matches := ix.Search(q)
	s := &Soil{Query: q.Text, Bed: q.Bed, Considered: ix.Len()}

	spent := 0
	for _, m := range matches {
		e := m.Entry
		head := fmt.Sprintf("%s %s %s", e.ID[:8], e.Type, e.Title)
		remaining := q.Budget - spent - tokens(head)
		if remaining < 20 {
			s.Omitted = len(matches) - len(s.Items)
			break
		}
		if remaining > q.Snippet {
			remaining = q.Snippet
		}
		snippet, partial := excerpt(e.Body, terms(q.Text), remaining)
		// Check the real cost rather than the predicted one. An excerpt has a
		// minimum useful length, so a snippet can come back slightly larger
		// than the space left for it; a budget that is only approximately
		// respected is not a budget.
		cost := tokens(head) + tokens(snippet)
		if spent+cost > q.Budget {
			s.Omitted = len(matches) - len(s.Items)
			break
		}
		item := Item{
			ID: e.ID, Type: string(e.Type), Scope: string(e.Scope), Bed: e.Bed,
			Title: e.Title, Snippet: snippet, Partial: partial,
			When: e.OccurredAt.Format("2006-01-02"),
		}
		if e.Review != "unreviewed" {
			item.Review = string(e.Review)
		}
		s.Items = append(s.Items, item)
		spent += cost
	}
	s.Tokens = spent
	return s
}

// excerpt returns the part of a body worth showing: the neighbourhood of the
// first query term, rather than the opening paragraph, because an entry's
// answer to a specific question is rarely its first sentence.
func excerpt(body string, q []string, budgetTokens int) (string, bool) {
	body = strings.TrimSpace(body)
	limit := budgetTokens * 4
	if limit < 80 {
		limit = 80
	}
	if len(body) <= limit {
		return body, false
	}

	lower := strings.ToLower(body)
	best := -1
	for _, t := range q {
		if i := strings.Index(lower, t); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return trimToWord(body, limit) + "…", true
	}

	start := best - limit/3
	if start < 0 {
		start = 0
	}
	if start > 0 {
		if i := strings.IndexAny(body[start:], " \n"); i >= 0 && i < 40 {
			start += i + 1
		}
	}
	end := start + limit
	if end > len(body) {
		end = len(body)
	}
	out := trimToWord(body[start:end], limit)
	if start > 0 {
		out = "…" + out
	}
	if end < len(body) {
		out += "…"
	}
	return out, true
}

func trimToWord(s string, limit int) string {
	if len(s) <= limit {
		s = strings.TrimSpace(s)
		return s
	}
	s = s[:limit]
	if i := strings.LastIndexAny(s, " \n"); i > limit/2 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// Render writes soil for a reader. Compact markdown, because it costs fewer
// tokens than JSON to say the same thing and every token here is paid for on
// every subsequent turn of whatever session receives it.
func (s *Soil) Render() string {
	if len(s.Items) == 0 {
		return fmt.Sprintf("The pile has nothing on %q (%d entries searched).\n", s.Query, s.Considered)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Soil for %q", s.Query)
	if s.Bed != "" {
		fmt.Fprintf(&b, " in %s", s.Bed)
	}
	fmt.Fprintf(&b, " — %d of %d entries, ~%d tokens.\n", len(s.Items), s.Considered, s.Tokens)
	b.WriteString("Ask for an entry by id to read it in full.\n")
	for _, it := range s.Items {
		fmt.Fprintf(&b, "\n## %s · %s", it.Title, it.Type)
		if it.Scope == "general" {
			b.WriteString(" · general")
		} else if it.Bed != s.Bed && it.Bed != "" {
			fmt.Fprintf(&b, " · from %s", it.Bed)
		}
		if it.Review != "" {
			fmt.Fprintf(&b, " · %s", it.Review)
		}
		fmt.Fprintf(&b, " · %s · id %s\n%s\n", it.When, it.ID[:8], it.Snippet)
	}
	if s.Omitted > 0 {
		fmt.Fprintf(&b, "\n%d further matches were left out to stay under budget.\n", s.Omitted)
	}
	return b.String()
}
