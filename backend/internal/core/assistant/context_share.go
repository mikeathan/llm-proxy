package assistant

// contextShare returns the character allowance for an injected block sized as
// a share of the model's resolved context budget (SPEC-005 §II.3), clamped to
// [lo, hi]; fallback is used when the budget is unresolved (<= 0). One home for
// the arithmetic so hot memory and the progress ledger scale identically.
func contextShare(contextBudget int, share float64, lo, hi, fallback int) int {
	if contextBudget <= 0 {
		return fallback
	}
	return min(max(int(float64(contextBudget)*share), lo), hi)
}
