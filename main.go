// deslop-go: a Go port of deslop-please's four structural lints
// (module-cycle, tangle, field-leak, shallow-module).
//
// Usage: deslop-go [-C dir] [-lint name,...] [-counts] [packages...]
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

var order = map[string]int{"module-cycle": 0, "tangle": 1, "field-leak": 2, "shallow-module": 3}

func main() {
	dir := flag.String("C", ".", "directory of the Go module")
	only := flag.String("lint", "", "comma-separated lints to run (default all)")
	countsOnly := flag.Bool("counts", false, "print only the counts by lint")
	all := flag.Bool("all", false, "list every tie and place in the notes, not only the first seven")
	flag.Parse()
	if *all {
		noteLimit = 1 << 30
	}
	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	m, _, err := load(*dir, patterns)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	g := newGraph(m)
	var ds []Diagnostic
	ds = append(ds, moduleCycles(m)...)
	ds = append(ds, tangles(m, g)...)
	ds = append(ds, fieldLeaks(m, g)...)
	ds = append(ds, shallow(m, g)...)
	if *only != "" {
		keep := map[string]bool{}
		for _, l := range strings.Split(*only, ",") {
			keep[l] = true
		}
		var f []Diagnostic
		for _, d := range ds {
			if keep[d.Lint] {
				f = append(f, d)
			}
		}
		ds = f
	}
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Lint != ds[j].Lint {
			return order[ds[i].Lint] < order[ds[j].Lint]
		}
		return ds[i].Key < ds[j].Key
	})
	counts := map[string]int{}
	for _, d := range ds {
		counts[d.Lint]++
		if *countsOnly {
			continue
		}
		fmt.Printf("error[%s]: %s\n", d.Lint, d.Message)
		if d.Location != nil {
			fmt.Printf("  --> %s\n", d.Location)
		}
		for _, n := range d.Notes {
			fmt.Printf("  = note: %s\n", n)
		}
		for _, p := range d.Picture {
			fmt.Printf("  %s\n", p)
		}
		fmt.Println()
	}
	var parts []string
	for _, l := range []string{"module-cycle", "tangle", "field-leak", "shallow-module"} {
		parts = append(parts, fmt.Sprintf("%s %d", l, counts[l]))
	}
	fmt.Printf("deslop-go: %d errors (%s)\n", len(ds), strings.Join(parts, ", "))
	if len(ds) > 0 {
		os.Exit(1)
	}
}
