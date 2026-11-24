package main

import (
	"path/filepath"
	"testing"
)

// Each fixture under testdata is a Go module built to trip one lint, mirroring deslop-please's own unit tests.
func TestFixtures(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    map[string][]string // lint -> keys
	}{
		// kinds/epoll uses socket, socket/address uses kinds: a cycle between kinds and socket. okchild and its
		// child package make none.
		{"cycle", map[string][]string{"module-cycle": {"kinds,socket"}}},
		// a ring of 8 types is too many to hold in mind; a ring of 7 is not
		{"tangle", map[string][]string{"tangle": {"ring::T8x0,ring::T8x1,ring::T8x2,ring::T8x3,ring::T8x4,ring::T8x5,ring::T8x6,ring::T8x7"}}},
		// Items is written outside, Spare's map reached through its getter; Head and Limit only read; n kept
		{"leak", map[string][]string{"field-leak": {"queue::RunQueue::Items", "queue::RunQueue::Spare"}}},
		// a closure field called, strings ranged over and a scalar read are reads; len() on a slice, a map index are not
		{"access", map[string][]string{"field-leak": {"box::Box::Ids", "box::Box::M"}}},
		// 8 shown of 16: shallow; 8 of 17 and 7 of 7: not
		{"shallow", map[string][]string{"shallow-module": {"storea"}}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			m, _, err := load(filepath.Join("testdata", tc.fixture), []string{"./..."})
			if err != nil {
				t.Fatal(err)
			}
			g := newGraph(m)
			var ds []Diagnostic
			ds = append(ds, moduleCycles(m)...)
			ds = append(ds, tangles(m, g)...)
			ds = append(ds, fieldLeaks(m, g)...)
			ds = append(ds, shallow(m, g)...)
			got := map[string][]string{}
			for _, d := range ds {
				got[d.Lint] = append(got[d.Lint], d.Key)
			}
			for lint, keys := range tc.want {
				if len(got[lint]) != len(keys) {
					t.Fatalf("%s: got %v, want %v", lint, got[lint], keys)
				}
				for _, k := range keys {
					found := false
					for _, g := range got[lint] {
						found = found || g == k
					}
					if !found {
						t.Errorf("%s: %q missing from %v", lint, k, got[lint])
					}
				}
			}
			if len(ds) != func() (n int) {
				for _, k := range tc.want {
					n += len(k)
				}
				return
			}() {
				t.Errorf("unexpected findings: %v", got)
			}
		})
	}
}
