package compost

import (
	"fmt"
	"strings"
)

// Render writes the digest in the form handed to the extractor: compact
// markdown rather than JSON, because it costs fewer tokens to say the same
// thing and models read it at least as well.
func (d *Digest) Render() string {
	var b strings.Builder

	fmt.Fprintf(&b, "# session %s\n", d.SessionID)
	fmt.Fprintf(&b, "bed: %s (%s)\n", d.Bed, d.Directory)
	if d.Branch != "" {
		fmt.Fprintf(&b, "branch: %s\n", d.Branch)
	}
	fmt.Fprintf(&b, "when: %s, lasting %s, %d tool calls\n",
		d.Start.Format("2006-01-02 15:04"), d.Duration.Round(60e9), d.ToolCalls)

	section(&b, "asked for", d.Asks, d.Truncated.Asks, "prompts")
	section(&b, "agent said", d.Notes, d.Truncated.Notes, "notes")

	if len(d.Edited) > 0 {
		b.WriteString("\n## changed\n")
		for _, f := range d.Edited {
			fmt.Fprintf(&b, "- %s (%d edits)\n", f.Path, f.Writes)
		}
	}
	if len(d.Read) > 0 {
		b.WriteString("\n## read\n")
		for _, f := range d.Read {
			fmt.Fprintf(&b, "- %s\n", f.Path)
		}
	}
	if d.Truncated.Files > 0 {
		fmt.Fprintf(&b, "- … %d more files\n", d.Truncated.Files)
	}

	if len(d.Commands) > 0 {
		b.WriteString("\n## ran\n")
		for _, c := range d.Commands {
			line := "- " + c.Command
			if c.Runs > 1 {
				line += fmt.Sprintf(" (×%d)", c.Runs)
			}
			if c.Errored > 0 {
				line += fmt.Sprintf(" [%d with stderr]", c.Errored)
			}
			b.WriteString(line + "\n")
		}
		if d.Truncated.Commands > 0 {
			fmt.Fprintf(&b, "- … %d more distinct commands\n", d.Truncated.Commands)
		}
	}

	if len(d.Records) > 0 {
		b.WriteString("\n## recorded\n")
		for _, r := range d.Records {
			if r.Bead != "" {
				fmt.Fprintf(&b, "- [%s] %s\n", r.Bead, r.Subject)
				continue
			}
			fmt.Fprintf(&b, "- %s\n", r.Subject)
		}
	}

	if len(d.Troubles) > 0 {
		b.WriteString("\n## went wrong\n")
		for _, t := range d.Troubles {
			fmt.Fprintf(&b, "- %s: %s\n", t.Where, t.Detail)
		}
	}

	return b.String()
}

func section(b *strings.Builder, title string, items []string, dropped int, unit string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", title)
	for _, s := range items {
		fmt.Fprintf(b, "- %s\n", s)
	}
	if dropped > 0 {
		fmt.Fprintf(b, "- … %d %s omitted from the middle\n", dropped, unit)
	}
}
