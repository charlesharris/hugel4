//go:build unix

package events

import "golang.org/x/sys/unix"

// writable answers whether this process could write at path, without writing
// there. It is the question Emit will ask the kernel a moment later, asked
// early so that health can tell "nothing has run" apart from "nothing could
// be recorded" -- two absences that look identical on disk.
//
// unix.Access tests the real uid rather than the effective one. For a CLI a
// gardener runs as themselves those are the same, and the alternative --
// creating a file to see whether creating a file works -- would make a
// read-only report write to the garden it is reporting on.
func writable(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}
