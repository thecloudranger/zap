package sizer

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Free returns the bytes available to the user on the filesystem holding path.
func Free(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("sizer: %w: %w", ErrNoStatfs, err)
	}
	return st.Bavail * uint64(st.Bsize), nil // Bsize's type differs by platform
}
