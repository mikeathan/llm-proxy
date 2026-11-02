// Command gen-oui rebuilds the bundled hardware-address registry from the IEEE file.
//
//	curl -o oui.csv https://standards-oui.ieee.org/oui/oui.csv
//	go run ./tools/gen-oui oui.csv internal/core/netaudit/oui/oui.tsv.gz
package main

import (
	"fmt"
	"os"

	"llm-proxy/internal/core/netaudit/oui"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gen-oui oui.csv out.tsv.gz")
		os.Exit(2)
	}
	in, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer in.Close()
	out, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := oui.Convert(in, out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := out.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
