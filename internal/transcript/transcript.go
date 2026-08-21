// Package transcript reads harness session transcripts into a form the rest of
// hugel can account for. Today the only source is Claude Code's JSONL session
// logs under ~/.claude/projects; the types here deliberately describe a
// *session* rather than a Claude Code file so other harnesses can be added
// without reshaping callers.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Usage is the token accounting for a single model request.
type Usage struct {
	Input          int `json:"input"`
	Output         int `json:"output"`
	Thinking       int `json:"thinking"`
	CacheRead      int `json:"cache_read"`
	CacheWrite5m   int `json:"cache_write_5m"`
	CacheWrite1h   int `json:"cache_write_1h"`
	WebSearchCalls int `json:"web_search_calls"`
}

// Add accumulates other into u.
func (u *Usage) Add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.Thinking += o.Thinking
	u.CacheRead += o.CacheRead
	u.CacheWrite5m += o.CacheWrite5m
	u.CacheWrite1h += o.CacheWrite1h
	u.WebSearchCalls += o.WebSearchCalls
}

// CacheWrite is total tokens written to cache at any TTL.
func (u Usage) CacheWrite() int { return u.CacheWrite5m + u.CacheWrite1h }

// ContextRead is the tokens the model re-read to have the conversation so far:
// fresh input plus cache hits. This is the number that grows with turn count.
func (u Usage) ContextRead() int { return u.Input + u.CacheRead }

// Request is one model API call. Claude Code emits several transcript records
// per request, all carrying the same usage snapshot, so requests are deduped by
// RequestID during parsing — summing raw records overstates cost badly.
type Request struct {
	RequestID string    `json:"request_id"`
	At        time.Time `json:"at"`
	Model     string    `json:"model"`
	Speed     string    `json:"speed"`  // "standard" or "fast"
	Effort    string    `json:"effort"` // low|medium|high|xhigh|max, when recorded
	Sidechain bool      `json:"sidechain"`
	Usage     Usage     `json:"usage"`
}

// Session is one harness session: a transcript file.
type Session struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	CWD      string    `json:"cwd"`
	Bed      string    `json:"bed"`
	Branch   string    `json:"branch"`
	Version  string    `json:"version"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Prompts  int       `json:"prompts"`
	Requests []Request `json:"-"`

	// Activity is what the session did, normalised and bounded. It is the raw
	// material the composter distils; nothing here is authoritative state.
	Asks  []Prompt  `json:"-"`
	Notes []Note    `json:"-"`
	Tools []ToolUse `json:"-"`
}

// Duration is wall-clock from first to last recorded event.
func (s Session) Duration() time.Duration { return s.End.Sub(s.Start) }

// Usage totals every request in the session.
func (s Session) Usage() Usage {
	var t Usage
	for _, r := range s.Requests {
		t.Add(r.Usage)
	}
	return t
}

// SidechainUsage totals only subagent requests — work that happened in a
// throwaway context and never entered the main thread.
func (s Session) SidechainUsage() Usage {
	var t Usage
	for _, r := range s.Requests {
		if r.Sidechain {
			t.Add(r.Usage)
		}
	}
	return t
}

// Models lists distinct models used, most-used first.
func (s Session) Models() []string {
	n := map[string]int{}
	for _, r := range s.Requests {
		if r.Model != "" {
			n[r.Model]++
		}
	}
	out := make([]string, 0, len(n))
	for m := range n {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if n[out[i]] != n[out[j]] {
			return n[out[i]] > n[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// --- wire format -----------------------------------------------------------

type rawUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	OutputTokensDetails      struct {
		ThinkingTokens int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
	CacheCreation struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	ServerToolUse struct {
		WebSearchRequests int `json:"web_search_requests"`
		WebFetchRequests  int `json:"web_fetch_requests"`
	} `json:"server_tool_use"`
	Speed string `json:"speed"`
}

func (r rawUsage) toUsage() Usage {
	u := Usage{
		Input:          r.InputTokens,
		Output:         r.OutputTokens,
		Thinking:       r.OutputTokensDetails.ThinkingTokens,
		CacheRead:      r.CacheReadInputTokens,
		CacheWrite5m:   r.CacheCreation.Ephemeral5m,
		CacheWrite1h:   r.CacheCreation.Ephemeral1h,
		WebSearchCalls: r.ServerToolUse.WebSearchRequests + r.ServerToolUse.WebFetchRequests,
	}
	// Older transcripts report only the flat cache_creation_input_tokens with no
	// TTL split. Attribute those to the 5m tier, the cheaper and far more common
	// default, so an unknown TTL never inflates the bill.
	if u.CacheWrite5m == 0 && u.CacheWrite1h == 0 && r.CacheCreationInputTokens > 0 {
		u.CacheWrite5m = r.CacheCreationInputTokens
	}
	return u
}

type rawRecord struct {
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	RequestID   string `json:"requestId"`
	SessionID   string `json:"sessionId"`
	CWD         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Version     string `json:"version"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	Effort      string `json:"effort"`
	Message     struct {
		Model   string          `json:"model"`
		Usage   *rawUsage       `json:"usage"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

// contentBlock is one element of a message's content array. Claude Code also
// writes plain-string content for simple turns, handled separately.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
}

// blocks decodes a message's content, which is either an array of blocks or a
// bare string for simple turns.
func blocks(raw json.RawMessage) []contentBlock {
	if len(raw) == 0 {
		return nil
	}
	var bs []contentBlock
	if err := json.Unmarshal(raw, &bs); err == nil {
		return bs
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		return []contentBlock{{Type: "text", Text: text}}
	}
	return nil
}

// toolResult is the union of result shapes hugel understands. Unknown shapes
// decode to zero values and are simply not described.
type toolResult struct {
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	Interrupted bool   `json:"interrupted"`
	FilePath    string `json:"filePath"`
	Type        string `json:"type"`
}

// toolInput is the union of tool argument shapes hugel summarises.
type toolInput struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
	Path        string `json:"path"`
	Pattern     string `json:"pattern"`
	Prompt      string `json:"prompt"`
	Skill       string `json:"skill"`
	Query       string `json:"query"`
}

// subject picks the most identifying argument for a tool call.
func (i toolInput) subject() string {
	for _, v := range []string{i.Command, i.FilePath, i.Path, i.Pattern, i.Query, i.Skill, i.Prompt} {
		if v != "" {
			return v
		}
	}
	return ""
}

// --- parsing ---------------------------------------------------------------

// ParseFile reads one transcript file.
func ParseFile(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()
	s, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s.Path = path
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if s.Bed == "" {
		s.Bed = bedFromSlug(filepath.Base(filepath.Dir(path)))
	}
	return s, nil
}

// Parse reads JSONL transcript records from r.
//
// Records are tolerated liberally: unknown types are ignored and a malformed
// line is skipped rather than failing the file, because a transcript is an
// append-only log that may be truncated mid-write by a live session.
func Parse(r io.Reader) (*Session, error) {
	s := &Session{}
	seen := make(map[string]bool)
	pending := make(map[string]int) // tool_use id -> index in s.Tools

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec rawRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.SessionID != "" && s.ID == "" {
			s.ID = rec.SessionID
		}
		if rec.CWD != "" && s.CWD == "" {
			s.CWD = rec.CWD
			s.Bed = filepath.Base(rec.CWD)
		}
		if rec.GitBranch != "" {
			s.Branch = rec.GitBranch
		}
		if rec.Version != "" {
			s.Version = rec.Version
		}
		if ts, err := time.Parse(time.RFC3339, rec.Timestamp); err == nil {
			if s.Start.IsZero() || ts.Before(s.Start) {
				s.Start = ts
			}
			if ts.After(s.End) {
				s.End = ts
			}
		}
		switch rec.Type {
		case "user":
			// isMeta marks injected/system-authored user turns, not something
			// the gardener typed.
			real := !rec.IsMeta && !rec.IsSidechain
			if real {
				s.Prompts++
			}
			at, _ := time.Parse(time.RFC3339, rec.Timestamp)
			for _, b := range blocks(rec.Message.Content) {
				switch b.Type {
				case "text":
					if real && strings.TrimSpace(b.Text) != "" {
						s.Asks = append(s.Asks, Prompt{At: at, Text: strings.TrimSpace(b.Text)})
					}
				case "tool_result":
					if i, ok := pending[b.ToolUseID]; ok {
						attachResult(&s.Tools[i], rec.ToolUseResult)
						delete(pending, b.ToolUseID)
					}
				}
			}
		case "assistant":
			at, _ := time.Parse(time.RFC3339, rec.Timestamp)
			for _, b := range blocks(rec.Message.Content) {
				switch b.Type {
				case "text":
					if t := strings.TrimSpace(b.Text); t != "" {
						s.Notes = append(s.Notes, Note{At: at, Text: t})
					}
				case "tool_use":
					var in toolInput
					_ = json.Unmarshal(b.Input, &in)
					s.Tools = append(s.Tools, ToolUse{
						At:     at,
						Name:   b.Name,
						Target: clip(in.subject(), excerptLimit),
						Detail: in.Description,
					})
					if b.ID != "" {
						pending[b.ID] = len(s.Tools) - 1
					}
				}
			}
			if rec.Message.Usage == nil {
				continue
			}
			key := rec.RequestID
			if key == "" {
				key = rec.UUID
			}
			if seen[key] {
				continue // same request, already counted
			}
			seen[key] = true
			u := rec.Message.Usage.toUsage()
			speed := rec.Message.Usage.Speed
			if speed == "" {
				speed = "standard"
			}
			s.Requests = append(s.Requests, Request{
				RequestID: key,
				At:        at,
				Model:     rec.Message.Model,
				Speed:     speed,
				Effort:    rec.Effort,
				Sidechain: rec.IsSidechain,
				Usage:     u,
			})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	sort.Slice(s.Requests, func(i, j int) bool { return s.Requests[i].At.Before(s.Requests[j].At) })
	return s, nil
}

// attachResult folds a tool's recorded outcome back onto its invocation.
// Claude Code does not record an exit code, so failure is inferred from an
// interrupt or from anything written to stderr — which is a hint, not a
// verdict, since plenty of healthy tools write to stderr.
func attachResult(t *ToolUse, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var r toolResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return
	}
	t.Output = clip(strings.TrimSpace(r.Stdout), excerptLimit)
	t.Stderr = clip(strings.TrimSpace(r.Stderr), excerptLimit)
	t.Errored = r.Interrupted || strings.TrimSpace(r.Stderr) != ""
	if t.Target == "" && r.FilePath != "" {
		t.Target = r.FilePath
	}
}

// bedFromSlug recovers a bed name from Claude Code's flattened project
// directory name (/Users/x/src/beer-run -> -Users-x-src-beer-run). The mapping
// is lossy — path separators and hyphens both become "-" — so this is only a
// fallback for transcripts with no recorded cwd.
func bedFromSlug(slug string) string {
	slug = strings.TrimPrefix(slug, "-")
	if i := strings.LastIndex(slug, "-"); i >= 0 && i < len(slug)-1 {
		return slug[i+1:]
	}
	return slug
}
