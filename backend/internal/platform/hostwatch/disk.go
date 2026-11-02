package hostwatch

import "syscall"

// Levels of DiskLow.
const (
	DiskLow      = 1 // under 10% free and under 50 GiB free
	DiskCritical = 2 // under 3% free or under 2 GiB free
)

const (
	lowFraction      = 10 // percent
	lowBytes         = 50 << 30
	criticalFraction = 3
	criticalBytes    = 2 << 30
)

// diskLevel judges one volume. A large volume is not a finding for being at 9%: the free bytes must be low too.
func diskLevel(free, total uint64) int {
	if total == 0 {
		return 0
	}
	pct := free * 100 / total
	switch {
	case pct < criticalFraction || free < criticalBytes:
		return DiskCritical
	case pct < lowFraction && free < lowBytes:
		return DiskLow
	}
	return 0
}

// diskLow returns the worst level over the configured volumes. It reads each volume's size and free space only: no file name,
// no content. A volume that cannot be read is counted, never reported as full.
func (h Hardener) diskLow(out *Hardening) {
	space := h.Space
	if space == nil {
		space = volumeSpace
	}
	for _, p := range h.DiskPaths {
		free, total, err := space(p)
		if err != nil {
			out.Unreadable++
			continue
		}
		out.DiskLow = max(out.DiskLow, diskLevel(free, total))
	}
}

// volumeSpace is the free space (what an ordinary account may use) and the size of the volume holding path.
func volumeSpace(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	size := uint64(st.Bsize) //nolint:gosec // a block size is never negative
	return uint64(st.Bavail) * size, uint64(st.Blocks) * size, nil
}
