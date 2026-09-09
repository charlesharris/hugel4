//go:build !unix

package events

// writable cannot be answered on this platform without writing, so it answers
// the way that changes nothing: health keeps the behaviour it had before the
// probe existed rather than demoting a garden this build cannot judge. A wrong
// "healthy" is the bug the probe exists to fix, but a wrong "unknown" on every
// garden would be a worse one.
//
// KNOWN LIMITATION, recorded rather than hidden: on a non-Unix build a garden
// that refuses writes, or a filesystem with no room left, still reports as
// healthy -- exactly the silent failure SUB-03 exists to remove. hugel targets
// macOS and Linux, there is no non-Unix CI, and nothing here is exercised by a
// test on those platforms. Closing it needs a Windows equivalent of
// unix.Access and unix.Statfs; until then README.md states the gap for anyone
// running a build from this file.
func writable(string) bool { return true }
