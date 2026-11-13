package sizer

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Free returns the bytes available to the user on the filesystem holding path.
func Free(path string) (uint64, error) {
	free, _, err := Disk(path)
	return free, err
}

// Disk returns the bytes available to the user and the total size of the filesystem
// holding path, as `df` reports them.
func Disk(path string) (free, total uint64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("sizer: %w: %w", ErrNoStatfs, err)
	}
	bs := uint64(st.Bsize) // Bsize's type differs by platform
	return st.Bavail * bs, st.Blocks * bs, nil
}

// ParseSize reads a size written the way Human prints one: "500MB", "1.5 GB", "10G",
// or a plain byte count. Units are decimal (1 GB = 1000^3 bytes); case is ignored.
func ParseSize(s string) (int64, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	num := strings.TrimRight(t, "BKMGTP ")
	unit := strings.TrimSpace(strings.TrimPrefix(t, num))
	mult := map[string]float64{"": 1, "B": 1, "K": 1e3, "KB": 1e3, "M": 1e6, "MB": 1e6,
		"G": 1e9, "GB": 1e9, "T": 1e12, "TB": 1e12, "P": 1e15, "PB": 1e15}[unit]
	f, err := strconv.ParseFloat(num, 64)
	if err != nil || mult == 0 || f < 0 {
		return 0, fmt.Errorf("sizer: %q is not a size (try 500MB, 1GB, 1.5TB)", s)
	}
	return int64(f * mult), nil
}
