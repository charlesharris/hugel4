// Package pricing converts token usage into dollars.
//
// Rates are Anthropic first-party API list prices, dollars per million tokens.
// They are a snapshot and will drift; hugel treats them as configuration that
// happens to have a compiled-in default, not as truth.
package pricing

import (
	"regexp"
	"strings"
)

// Rate is the per-million-token price for a model.
type Rate struct {
	Input  float64
	Output float64
}

// Cache multipliers apply to the model's input rate.
const (
	CacheReadMult    = 0.10 // reading a cache hit
	CacheWrite5mMult = 1.25 // writing with the default 5-minute TTL
	CacheWrite1hMult = 2.00 // writing with the 1-hour TTL
)

// rates as of 2026-08. Keep sorted by family.
var rates = map[string]Rate{
	"claude-fable-5":    {Input: 10, Output: 50},
	"claude-mythos-5":   {Input: 10, Output: 50},
	"claude-opus-5":     {Input: 5, Output: 25},
	"claude-opus-4-8":   {Input: 5, Output: 25},
	"claude-opus-4-7":   {Input: 5, Output: 25},
	"claude-opus-4-6":   {Input: 5, Output: 25},
	"claude-sonnet-5":   {Input: 3, Output: 15},
	"claude-sonnet-4-6": {Input: 3, Output: 15},
	"claude-haiku-4-5":  {Input: 1, Output: 5},
}

// fastRates apply when a request ran in fast mode, which is billed at a premium.
var fastRates = map[string]Rate{
	"claude-opus-5":   {Input: 10, Output: 50},
	"claude-opus-4-8": {Input: 10, Output: 50},
}

var dateSuffix = regexp.MustCompile(`-\d{8}$`)

// Normalize reduces a model string to the key used in the rate tables:
// strips a bracketed context variant ("claude-opus-5[1m]") and any trailing
// date snapshot ("claude-sonnet-4-5-20250929").
func Normalize(model string) string {
	m := strings.TrimSpace(strings.ToLower(model))
	if i := strings.IndexByte(m, '['); i >= 0 {
		m = m[:i]
	}
	m = dateSuffix.ReplaceAllString(m, "")
	return m
}

// free models are recorded in transcripts but never billed: Claude Code's
// "<synthetic>" placeholder for locally generated assistant turns (interrupts,
// errors) and records with no model at all.
var free = map[string]bool{"": true, "<synthetic>": true, "<none>": true}

// Free reports whether a model is a non-billable placeholder rather than a
// model hugel simply has no price for.
func Free(model string) bool { return free[Normalize(model)] }

// Lookup returns the rate for a model at a given speed. ok is false for models
// hugel has no price for, so callers can report unpriced usage rather than
// silently valuing it at zero.
func Lookup(model, speed string) (Rate, bool) {
	m := Normalize(model)
	if speed == "fast" {
		if r, ok := fastRates[m]; ok {
			return r, true
		}
	}
	r, ok := rates[m]
	return r, ok
}

// Cost breaks a request's spend into the four things you actually pay for.
type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

// Total is the whole bill.
func (c Cost) Total() float64 { return c.Input + c.Output + c.CacheRead + c.CacheWrite }

// Context is what was spent carrying the conversation rather than producing
// new work: fresh input, cache hits, and the writes that made those hits
// possible.
func (c Cost) Context() float64 { return c.Input + c.CacheRead + c.CacheWrite }

// Add accumulates other into c.
func (c *Cost) Add(o Cost) {
	c.Input += o.Input
	c.Output += o.Output
	c.CacheRead += o.CacheRead
	c.CacheWrite += o.CacheWrite
}

// Tokens is the usage shape pricing needs. It mirrors transcript.Usage without
// importing it, so pricing stays independent of any transcript format.
type Tokens struct {
	Input        int
	Output       int
	CacheRead    int
	CacheWrite5m int
	CacheWrite1h int
}

// Price values usage at the given model and speed.
func Price(model, speed string, t Tokens) (Cost, bool) {
	r, ok := Lookup(model, speed)
	if !ok {
		return Cost{}, false
	}
	const perToken = 1e6
	return Cost{
		Input:      float64(t.Input) * r.Input / perToken,
		Output:     float64(t.Output) * r.Output / perToken,
		CacheRead:  float64(t.CacheRead) * r.Input * CacheReadMult / perToken,
		CacheWrite: (float64(t.CacheWrite5m)*CacheWrite5mMult + float64(t.CacheWrite1h)*CacheWrite1hMult) * r.Input / perToken,
	}, true
}
