//go:build !unix

package events

// writable cannot be answered here without writing, so it answers the way
// that changes nothing: health keeps the behaviour it had before the probe
// existed rather than demoting a garden this platform cannot judge. A wrong
// "healthy" is the bug this probe exists to fix, but a wrong "unknown" on
// every garden would be a worse one.
func writable(string) bool { return true }
