package transcript

import "time"

// excerptLimit bounds how much of any single tool payload is retained. A
// transcript can carry megabytes of command output; hugel only ever needs
// enough to recognise what happened, and holding the rest would make the
// distiller's job harder rather than easier.
const excerptLimit = 800

// commandLimit is larger because a command is sometimes the record itself. A
// commit message written through a heredoc carries the reasoning for a change,
// which is exactly the material worth composting; clipping it at the output
// limit would truncate the knowledge and keep the noise.
const commandLimit = 4000

// Prompt is something the gardener actually asked for. These are the highest
// signal and lowest volume records in a transcript.
type Prompt struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

// Note is prose the agent wrote to the gardener — its own account of what it
// was doing. Tool calls say what happened; notes say why.
type Note struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

// ToolUse is one tool invocation, normalised across tools and bounded in size.
type ToolUse struct {
	At      time.Time `json:"at"`
	Name    string    `json:"name"`
	Target  string    `json:"target,omitempty"`  // file path, command, or subject
	Detail  string    `json:"detail,omitempty"`  // the agent's own description
	Output  string    `json:"output,omitempty"`  // bounded excerpt of stdout
	Errored bool      `json:"errored,omitempty"` // stderr present or interrupted
	Stderr  string    `json:"stderr,omitempty"`  // bounded excerpt
}

// Reads reports whether the tool call only observed the workspace.
func (t ToolUse) Reads() bool {
	switch t.Name {
	case "Read", "Grep", "Glob", "NotebookRead":
		return true
	}
	return false
}

// Writes reports whether the tool call changed a file.
func (t ToolUse) Writes() bool {
	switch t.Name {
	case "Edit", "Write", "NotebookEdit":
		return true
	}
	return false
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
