package compost

import "github.com/charris/hugel/internal/redact"

// Redact scrubs every text field in the digest and records what was found.
//
// This runs on the digest rather than on the transcript because the digest is
// the only thing that ever leaves memory. A transcript already sits on disk
// under the harness's own control; the pile is hugel's, permanent, and shared
// across every bed, so this is the boundary that matters.
func (d *Digest) Redact(r *redact.Redactor) []redact.Hit {
	counts := map[redact.Class]int{}
	collect := func(hits []redact.Hit) {
		for _, h := range hits {
			counts[h.Class] += h.Count
		}
	}

	var hits []redact.Hit
	d.Asks, hits = r.Strings(d.Asks)
	collect(hits)
	d.Notes, hits = r.Strings(d.Notes)
	collect(hits)

	for i := range d.Commands {
		d.Commands[i].Command, hits = r.Redact(d.Commands[i].Command)
		collect(hits)
	}
	for i := range d.Records {
		d.Records[i].Subject, hits = r.Redact(d.Records[i].Subject)
		collect(hits)
		d.Records[i].Body, hits = r.Redact(d.Records[i].Body)
		collect(hits)
	}
	for i := range d.Troubles {
		d.Troubles[i].Where, hits = r.Redact(d.Troubles[i].Where)
		collect(hits)
		d.Troubles[i].Detail, hits = r.Redact(d.Troubles[i].Detail)
		collect(hits)
	}
	// Paths can carry credentials too — a checkout under a token-bearing URL,
	// a dotfile named for its secret.
	for i := range d.Edited {
		d.Edited[i].Path, hits = r.Redact(d.Edited[i].Path)
		collect(hits)
	}
	for i := range d.Read {
		d.Read[i].Path, hits = r.Redact(d.Read[i].Path)
		collect(hits)
	}

	out := make([]redact.Hit, 0, len(counts))
	for c, n := range counts {
		out = append(out, redact.Hit{Class: c, Count: n})
	}
	sortHits(out)
	d.Redactions = out
	return out
}

func sortHits(h []redact.Hit) {
	for i := 1; i < len(h); i++ {
		for j := i; j > 0 && h[j].Class < h[j-1].Class; j-- {
			h[j], h[j-1] = h[j-1], h[j]
		}
	}
}
