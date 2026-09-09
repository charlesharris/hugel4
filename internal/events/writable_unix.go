//go:build unix

package events

import "golang.org/x/sys/unix"

// platformPermitsWrite asks the kernel whether this process may write at path,
// without writing there.
//
// unix.Access tests the real uid rather than the effective one. For a CLI a
// gardener runs as themselves those are the same, and the alternative --
// creating a file to see whether creating a file works -- would make a
// read-only report write to the garden it is reporting on.
func platformPermitsWrite(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}

// platformBlocksAvailable reports the blocks an unprivileged process could
// still use on the filesystem holding path. Reserved space only root can reach
// is not space this write can use, and Bavail already excludes it.
//
// The error is not a failure to report: it means the filesystem declined to be
// interrogated, which is not evidence of anything. Callers treat it as "no
// second opinion" rather than as bad news.
func platformBlocksAvailable(path string) (uint64, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return 0, err
	}
	return uint64(fs.Bavail), nil
}
