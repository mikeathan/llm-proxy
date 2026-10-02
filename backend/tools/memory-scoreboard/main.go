// memory-scoreboard prints the memory scoreboard for one or more run
// directories (docs/PLANS/memory/small-context-memory.md, Phase 0): memory
// tokens per request, derived sieve firings, repeated (tool,args) calls,
// turn-1 latency and steps. It reads only files the runtime already records
// (events.jsonl, recording.jsonl, run-meta.json) and prints n/a for anything
// it cannot derive. Stdlib only.
//
//	cd backend && go run ./tools/memory-scoreboard/ <run-dir>...
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: memory-scoreboard <run-dir>...")
		os.Exit(2)
	}
	var rows []Row
	failed := false
	for _, dir := range os.Args[1:] {
		row, err := Analyze(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "skip:", err)
			failed = true
			continue
		}
		rows = append(rows, row)
	}
	fmt.Print(FormatRows(rows))
	if failed {
		os.Exit(1)
	}
}
