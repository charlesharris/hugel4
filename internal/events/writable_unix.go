//go:build unix

package events

import "golang.org/x/sys/unix"

// writable answers whether a write at path could succeed, without writing
// there. It is the question Emit will ask the kernel a moment later, asked
// early so that health can tell "nothing has run" apart from "nothing could
// be recorded" -- two absences that look identical on disk.
//
// unix.Access tests the real uid rather than the effective one. For a CLI a
// gardener runs as themselves those are the same, and the alternative --
// creating a file to see whether creating a file works -- would make a
// read-only report write to the garden it is reporting on.
func writable(path string) bool {
	if unix.Access(path, unix.W_OK) != nil {
		return false
	}
	// Permission is not the only way a write gets refused, and the other way
	// is the one Success Criterion 1 names by name: a full disk. Access reads
	// mode bits and knows nothing about free space, so a garden on a
	// filesystem with nothing left reads as perfectly writable right up until
	// Emit comes back with ENOSPC -- and markFailing cannot record that
	// either, because a marker needs an inode too.
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		// The filesystem declined to be interrogated. Permission already said
		// yes, and a missing second opinion is not evidence of a problem:
		// manufacturing a failure here would put "unknown" on healthy gardens.
		return true
	}
	// Blocks available to an unprivileged process, which is what a gardener
	// is. Reserved space that only root can reach is not space this write can
	// use, and Bavail already excludes it.
	return fs.Bavail > 0
}
