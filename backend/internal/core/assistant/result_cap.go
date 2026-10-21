package assistant

import "llm-proxy/internal/core/proxy"

// A tool result used to be cut to a fixed 3000 characters whatever the model and window. On a 16k local model that is a
// small slice of a window with about 17k characters of room, and it cut every internet_search result (5.5 to 7.4 KB)
// to half, and made a 7 KB file take three reads, which a model sometimes stopped after two (4 of 8 complete) while it
// listed all of it when the file came whole (8 of 8). The cap is now sized per tool turn from the room left in the
// request, with today's 3000 as the floor, so no result is ever smaller than before.
const (
	// resultCapShare is the share of the room left that one turn's results may take between them: each result takes
	// half of what is left, so the caps shrink as the request fills and their total can never reach the room.
	resultCapShare = 0.5
	// resultCapCeiling bounds one result on a big window, where room is not the limit but reading is.
	resultCapCeiling = 12000
)

// resultCapFor is how many characters one tool result may keep for a turn that makes `calls` calls on a request of the
// given size: the share of the room left, split over the calls, between the floor (proxy.MaxReturnChars) and the ceiling.
// With no budget known (size.limit is 0) it is the floor, which is today's behaviour.
func resultCapFor(size requestSize, calls int) int {
	floor := proxy.MaxReturnChars
	room := size.limit - size.chars
	if size.limit <= 0 || room <= 0 {
		return floor
	}
	perCall := int(float64(room)*resultCapShare) / max(calls, 1)
	return min(max(perCall, floor), resultCapCeiling)
}

// noteRequestSize records the size of the request about to be sent: the basis of the cap on the results that follow it.
func (a *Agent) noteRequestSize(size requestSize) {
	if a.runS != nil {
		a.runS.requestSize = size
	}
}

// beginResultCap fixes the cap for the calls of one tool turn, once, before any of them runs, so every call (and the
// result appended for it) uses the same value however many results the turn has already added. With the switch off, or
// no budget known, it is the default (see resultCapFor).
func (a *Agent) beginResultCap(calls int) {
	if a.runS == nil {
		return
	}
	a.runS.toolCap = 0
	if !a.config.AdaptiveResultCap {
		return
	}
	a.runS.toolCap = resultCapFor(a.runS.requestSize, calls)
	a.deps.Logger.Debug("result cap for this turn", "calls", calls, "cap", a.runS.toolCap, "request", a.runS.requestSize.chars, "limit", a.runS.requestSize.limit)
}

// endResultCap ends the turn's cap: a result appended outside a tool turn gets the default.
func (a *Agent) endResultCap() {
	if a.runS != nil {
		a.runS.toolCap = 0
	}
}

// currentResultCap is the cap on one result now: the turn's, or the default outside a tool turn.
func (a *Agent) currentResultCap() int {
	if a.runS != nil && a.runS.toolCap > 0 {
		return a.runS.toolCap
	}
	return proxy.MaxReturnChars
}
