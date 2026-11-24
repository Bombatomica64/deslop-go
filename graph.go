package main

// Who knows what (deslop-please's src/graph). A thing is a type with its
// fields and methods, a free function, or a package-level var or const. A
// use is a reference from non-test code to an item of a non-test thing, seen
// through wrappers. Things that know each other in a circle are a tangle.

import "sort"

type Thing struct {
	Item int
	Home int
}

type Use struct {
	Reference int
	User      int // thing, -1 for none
	Item      int
	Through   int // the wrapper the reference names, -1 for none
	Copies    bool
}

type Graph struct {
	Things   []Thing
	ThingOf  []int // by item, -1 for none
	Uses     []Use
	Tangles  [][]int
	TangleOf []int // by thing, -1 for none
}

func (m *Model) counted(item int) bool {
	return !m.Tree.Nodes[m.Items[item].Owner].Test
}

func newGraph(m *Model) *Graph {
	g := &Graph{ThingOf: make([]int, len(m.Items))}
	ofRoot := map[int]int{}
	for i := range m.Items {
		g.ThingOf[i] = -1
		root := i
		for m.Items[root].Parent >= 0 {
			root = m.Items[root].Parent
		}
		if !m.counted(i) || !m.counted(root) {
			continue
		}
		t, ok := ofRoot[root]
		if !ok {
			g.Things = append(g.Things, Thing{Item: root, Home: m.Items[root].Owner})
			t = len(g.Things) - 1
			ofRoot[root] = t
		}
		g.ThingOf[i] = t
	}
	for ri, r := range m.References {
		if m.Tree.Nodes[r.From].Test {
			continue
		}
		if r.Holder >= 0 {
			if _, w := m.Wrappers[r.Holder]; w {
				continue
			}
		}
		user := -1
		if r.Holder >= 0 {
			user = g.ThingOf[r.Holder]
		}
		through := -1
		if _, w := m.Wrappers[r.Item]; w {
			through = r.Item
		}
		for _, b := range behind(m, r) {
			if g.ThingOf[b.item] >= 0 {
				g.Uses = append(g.Uses, Use{Reference: ri, User: user, Item: b.item, Through: through, Copies: b.copies})
			}
		}
	}
	knows := make([][]int, len(g.Things))
	for _, u := range g.Uses {
		used := g.ThingOf[u.Item]
		if u.User >= 0 && used >= 0 && u.User != used {
			knows[u.User] = append(knows[u.User], used)
		}
	}
	g.Tangles = circles(knows)
	g.TangleOf = make([]int, len(g.Things))
	for i := range g.TangleOf {
		g.TangleOf[i] = -1
	}
	for ti, members := range g.Tangles {
		for _, mm := range members {
			g.TangleOf[mm] = ti
		}
	}
	return g
}

type behindItem struct {
	item   int
	copies bool
}

func behind(m *Model, r Reference) []behindItem {
	var used []behindItem
	at := r.Item
	for n := 0; n < 1000; n++ {
		w, ok := m.Wrappers[at]
		if !ok {
			if at != r.Item {
				return append(used, behindItem{at, false})
			}
			return append(used, behindItem{at, m.Items[at].Kind == KField && onlyReads(r.Access, m.Items[at].Obj.Type())})
		}
		switch w.Kind {
		case Forwarder:
			for _, f := range w.Fields {
				used = append(used, behindItem{f, true})
			}
			at = w.Target
		case Accessor:
			// What a getter lends is read as the field would be; its result
			// taken as it is, is borrowed.
			a := r.Access
			if a == Copy && r.Plain {
				a = Borrow
			}
			for _, f := range w.Lent {
				used = append(used, behindItem{f, onlyReads(a, m.Items[f].Obj.Type())})
			}
			for _, f := range w.Copied {
				used = append(used, behindItem{f, true})
			}
			for _, f := range w.Written {
				used = append(used, behindItem{f, false})
			}
			return used
		}
	}
	return used
}

// circles: each strongly connected component of two or more nodes, sorted
// (Tarjan's algorithm, iterative).
func circles(edges [][]int) [][]int {
	const unseen = -1
	n := len(edges)
	index := make([]int, n)
	low := make([]int, n)
	on := make([]bool, n)
	for i := range index {
		index[i] = unseen
	}
	var stack []int
	next := 0
	var found [][]int
	type frame struct{ node, edge int }
	for start := 0; start < n; start++ {
		if index[start] != unseen {
			continue
		}
		work := []frame{{start, 0}}
		index[start], low[start] = next, next
		next++
		stack = append(stack, start)
		on[start] = true
		for len(work) > 0 {
			top := &work[len(work)-1]
			node := top.node
			if top.edge < len(edges[node]) {
				to := edges[node][top.edge]
				top.edge++
				if index[to] == unseen {
					index[to], low[to] = next, next
					next++
					stack = append(stack, to)
					on[to] = true
					work = append(work, frame{to, 0})
				} else if on[to] && index[to] < low[node] {
					low[node] = index[to]
				}
				continue
			}
			work = work[:len(work)-1]
			if len(work) > 0 {
				p := work[len(work)-1].node
				if low[node] < low[p] {
					low[p] = low[node]
				}
			}
			if low[node] == index[node] {
				var comp []int
				for {
					mm := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					on[mm] = false
					comp = append(comp, mm)
					if mm == node {
						break
					}
				}
				if len(comp) > 1 {
					sort.Ints(comp)
					found = append(found, comp)
				}
			}
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i][0] < found[j][0] })
	return found
}
