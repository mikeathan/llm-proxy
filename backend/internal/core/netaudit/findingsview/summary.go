package findingsview

import (
	"fmt"
	"strings"

	"llm-proxy/internal/core/netaudit"
)

// NoCheckFindings is what a summary says when the checks established nothing. An open port is a fact, not a problem; a model
// told only a total of "findings" counted ports as problems and named checks it had read about in a tool description.
const NoCheckFindings = "No check found a problem in this scan: do not name one."

// Line counts the open ports apart from what the checks established, and names each check once, in fixed words, with its
// count, in the order first seen. Both network_scan and network_findings open their result with it, so the model reads one wording.
func Line(findings []netaudit.Finding) string {
	ports, checks := 0, 0
	counts := map[string]int{}
	var order []string
	for _, f := range findings {
		if f.Check == netaudit.CheckOpenPort {
			ports++
			continue
		}
		checks++
		if counts[f.Check] == 0 {
			order = append(order, f.Check)
		}
		counts[f.Check]++
	}
	line := fmt.Sprintf("%d open port(s).", ports)
	if checks == 0 {
		return line + " " + NoCheckFindings
	}
	named := make([]string, 0, len(order))
	for _, c := range order {
		title := netaudit.CheckTitle(c)
		if title == "" {
			title = c
		}
		named = append(named, fmt.Sprintf("%s (%d)", title, counts[c]))
	}
	return fmt.Sprintf("%s %d check finding(s): %s.", line, checks, strings.Join(named, "; "))
}
