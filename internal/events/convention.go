package events

// The observability convention, stated once and read by both briefs.
//
// This is the practice rather than the package. It is about the beds hugel
// tends, not about hugel: tourdesource and hellbox cannot be made to adopt
// anything, and asking them to would be a project rather than a criterion.
// What can be done is that work landing in them arrives instrumented, because
// the reviewer refused it otherwise.
//
// It rides on the mechanism that puts acceptance criteria in front of the
// reviewer, which is why it costs almost nothing. A convention that depends on
// an agent remembering it is a convention that holds until the agent is busy.

// Convention is what a tender is told to do. It states the shape rather than
// naming a library, because the beds do not share one and the shape is the
// part that matters: one wide event per unit of work, carrying what was known
// when the work finished.
const Convention = `Code you write that does real work should say what it did. One event per unit
of work -- a request served, a job run, a decision taken -- emitted where the
work finishes, carrying what was known by then rather than a count of it.

Wide and flat: put the identifiers, the inputs that mattered, the outcome in
one word, and how long it took, in one record. Not several narrow log lines
that a reader has to join back together, and not a metric that has already
thrown away the reason.

Use whatever the project already uses. The shape is the convention; the
library is not. If the project has no such thing, a single structured line is
enough, and saying so in your result is better than inventing a framework.

Nothing to instrument is a fine answer. Tests, documentation and refactors
that change no behaviour add no new unit of work, and padding them with events
is worse than leaving them alone.`

// Criterion is what the reviewer answers, phrased so it can come out either
// way. A criterion nothing can fail is decoration, and one that fails every
// documentation change is noise that gets switched off within a week -- so it
// names the case where it does not apply as explicitly as the case where it
// does.
const Criterion = `Work that adds or changes behaviour emits a wide structured event where that
work finishes: one record per unit of work, carrying the identifiers, the
outcome in one word, and the duration.

This one is standing -- it holds for every bead, whether or not the bead states
it. Answer it like the others.

When the change adds no new unit of work, answer "met, nothing to instrument":
tests, documentation and refactors that change no behaviour meet it that way
rather than failing it. Say which of the two you mean. Both are passes, and
only one of them is a claim about the code.`
