package netaudit

import (
	"context"
	"fmt"
)

// ModuleTally is what one background module did over the last 24 hours, in counts. The words are the core's own (the module sends
// numbers only, ADR-0036), so nothing a resolver or a device wrote can reach a report through it.
type ModuleTally struct {
	// Title names the module, Noun what it counts ("checks") and FlaggedWord what a flagged one means ("differed").
	Title       string `json:"title"`
	Noun        string `json:"noun"`
	FlaggedWord string `json:"flagged_word"`
	Done        int    `json:"done"`
	Flagged     int    `json:"flagged"`
	Unclear     int    `json:"unclear"`
}

// TallySource lists the tallies of the modules that keep one. A module that did nothing in the period is not in the list.
type TallySource interface {
	Tallies(ctx context.Context) []ModuleTally
}

// Line is the tally as one sentence: "Router DNS check, last 24 h: 24 checks, 0 differed, 0 could not be compared."
func (t ModuleTally) Line() string {
	return fmt.Sprintf("%s, last 24 h: %d %s, %d %s, %d could not be compared.", t.Title, t.Done, t.Noun, t.Flagged, t.FlaggedWord, t.Unclear)
}
