//go:build linux

package recorder

import (
	"os"

	"golang.org/x/sys/unix"
)

// fadviseDropCache tells the kernel to evict this file's pages from the page
// cache. Called after sealing a completed recording segment so that write-once
// media data does not accumulate in memory indefinitely.
func fadviseDropCache(f *os.File) {
	unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED) //nolint:errcheck
}
