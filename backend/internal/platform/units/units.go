// Package units converts operator-facing size units to bytes. One home for the
// binary conversions (KiB/MiB/GiB) so config consumers cannot drift apart with
// hand-rolled 1024/<<20/<<30 arithmetic (repo rule: shared utilities live in
// one place; no magic numbers).
package units

const (
	// binary multipliers
	kib = 1 << 10
	mib = 1 << 20
	gib = 1 << 30
)

// KiB converts KiB to bytes; non-positive input yields 0 (fail-safe: an unset
// or invalid limit must never become a huge one).
func KiB(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * kib
}

// MiB converts MiB to bytes; non-positive input yields 0.
func MiB(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * mib
}

// GiB converts GiB to bytes; non-positive input yields 0.
func GiB(n int) int64 {
	if n <= 0 {
		return 0
	}
	return int64(n) * gib
}
