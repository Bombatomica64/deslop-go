package main

// The four lints of deslop-please (src/lint): module-cycle, tangle,
// field-leak and shallow-module, with the same thresholds.

import (
	"fmt"
	"sort"
	"strings"
)

// HeldInMind: what a reader can hold in mind at once (Miller, 1956).
const HeldInMind = 7

// BreakEven: what a module hides for each function it shows, at break-even.
const BreakEven = 1

type Location struct {
	File string
	Line int
}

func (l Location) String() string { return fmt.Sprintf("%s:%d:1", l.File, l.Line) }

func less(a, b Location) bool {
	if a.File != b.File {
		return a.File < b.File
	}
	return a.Line < b.Line
}

type Diagnostic struct {
	Lint     string
	Key      string
	Message  string
	Location *Location
	Notes    []string
	Picture  []string
}

func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

var noteLimit = HeldInMind

func listed(names []string) string {
	if len(names) <= noteLimit {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:noteLimit], ", ") + fmt.Sprintf(", +%d more", len(names)-noteLimit)
}

// ---- ties, shared by module-cycle and tangle ----

type Tie struct {
	items   map[int]bool
	holders map[int]bool
	sites   []Location
	written map[int]Location
}

func newTie() *Tie {
	return &Tie{items: map[int]bool{}, holders: map[int]bool{}, written: map[int]Location{}}
}

func (t *Tie) add(item int, r Reference, module int) {
	t.items[item] = true
	if r.Holder >= 0 {
		t.holders[r.Holder] = true
	}
	site := Location{r.File, r.Line}
	if first, ok := t.written[module]; !ok || less(site, first) {
		t.written[module] = site
	}
	t.sites = append(t.sites, site)
}

func (t *Tie) site() Location {
	best := t.sites[0]
	for _, s := range t.sites {
		if less(s, best) {
			best = s
		}
	}
	return best
}

func names(m *Model, set map[int]bool) []string {
	var out []string
	for i := range set {
		out = append(out, m.Items[i].Name)
	}
	sort.Strings(out)
	return out
}

func (t *Tie) note(m *Model, verb, user, used string) string {
	site := t.site()
	firsts := map[Location]bool{}
	for _, l := range t.written {
		firsts[l] = true
	}
	var elsewhere []string
	for l := range firsts {
		if l != site {
			elsewhere = append(elsewhere, l.String())
		}
	}
	sort.Strings(elsewhere)
	also := ""
	if len(elsewhere) > 0 {
		also = "; also at " + listed(elsewhere)
	}
	return fmt.Sprintf("%s: `%s` %s `%s` by %s: %s%s", site, user, verb, used,
		counted(len(t.items), "item"), listed(names(m, t.items)), also)
}

type namedTie struct {
	user, used string
	tie        *Tie
}

func circleReport(m *Model, verb, noun string, members int, ties []namedTie) (*Location, []string, []string) {
	sort.Slice(ties, func(i, j int) bool {
		a, b := ties[i], ties[j]
		if len(a.tie.items) != len(b.tie.items) {
			return len(a.tie.items) < len(b.tie.items)
		}
		if a.user != b.user {
			return a.user < b.user
		}
		return a.used < b.used
	})
	var notes []string
	for i, t := range ties {
		if i == noteLimit {
			notes = append(notes, "and "+counted(len(ties)-noteLimit, "more tie"))
			break
		}
		notes = append(notes, t.tie.note(m, verb, t.user, t.used))
	}
	circle := shortestCircle(ties)
	var pic []string
	for _, at := range circle {
		t := ties[at]
		hs := names(m, t.tie.holders)
		uses := "uses " + listed(names(m, t.tie.items))
		if len(hs) == 1 {
			uses = hs[0] + " " + uses
		}
		pic = append(pic, "  "+t.user, "    ┃ "+uses+"  ("+t.tie.site().String()+")", "    ▼")
	}
	if len(circle) > 0 {
		pic = append(pic, "  "+ties[circle[0]].user)
	}
	var left []string
	if members-len(circle) > 0 {
		left = append(left, counted(members-len(circle), "more "+noun))
	}
	if len(ties)-len(circle) > 0 {
		left = append(left, counted(len(ties)-len(circle), "more tie"))
	}
	if len(left) > 0 {
		pic = append(pic, "  not drawn: "+strings.Join(left, ", "))
	}
	var loc *Location
	if len(ties) > 0 {
		s := ties[0].tie.site()
		loc = &s
	}
	return loc, notes, pic
}

func shortestCircle(ties []namedTie) []int {
	start, end := ties[0].user, ties[0].used
	reached := map[string]int{end: -1}
	queue := []string{end}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		if at == start {
			break
		}
		for i, t := range ties {
			if t.user == at {
				if _, ok := reached[t.used]; !ok {
					reached[t.used] = i
					queue = append(queue, t.used)
				}
			}
		}
	}
	var circle []int
	at := start
	for {
		i, ok := reached[at]
		if !ok || i < 0 {
			break
		}
		circle = append(circle, i)
		at = ties[i].user
	}
	circle = append(circle, 0)
	for i, j := 0, len(circle)-1; i < j; i, j = i+1, j-1 {
		circle[i], circle[j] = circle[j], circle[i]
	}
	return circle
}

// ---- module-cycle ----

func moduleCycles(m *Model) []Diagnostic {
	tr := m.Tree
	ties := map[int]map[[2]int]*Tie{}
	for _, r := range m.References {
		it := m.Items[r.Item]
		if tr.Nodes[r.From].Test || tr.Nodes[it.Owner].Test {
			continue
		}
		parent, user, used, ok := siblings(tr, r.From, it.Owner)
		if !ok {
			continue
		}
		if ties[parent] == nil {
			ties[parent] = map[[2]int]*Tie{}
		}
		k := [2]int{user, used}
		if ties[parent][k] == nil {
			ties[parent][k] = newTie()
		}
		ties[parent][k].add(r.Item, r, user)
	}
	var out []Diagnostic
	for parent, ts := range ties {
		var children []int
		seen := map[int]bool{}
		for k := range ts {
			for _, c := range k {
				if !seen[c] {
					seen[c] = true
					children = append(children, c)
				}
			}
		}
		sort.Ints(children)
		place := map[int]int{}
		for i, c := range children {
			place[c] = i
		}
		uses := make([][]int, len(children))
		for k := range ts {
			uses[place[k[0]]] = append(uses[place[k[0]]], place[k[1]])
		}
		for _, circle := range circles(uses) {
			var members []int
			for _, at := range circle {
				members = append(members, children[at])
			}
			out = append(out, cycleDiagnostic(m, parent, members, ts))
		}
	}
	return out
}

func siblings(tr *Tree, a, b int) (parent, ca, cb int, ok bool) {
	parent = tr.commonAncestor(a, b)
	if parent == a || parent == b || tr.Nodes[parent].Kind == Workspace {
		return 0, 0, 0, false
	}
	child := func(of int) int {
		for _, n := range tr.ancestors(of) {
			if tr.Nodes[n].Parent == parent {
				return n
			}
		}
		return -1
	}
	ca, cb = child(a), child(b)
	return parent, ca, cb, ca >= 0 && cb >= 0
}

func cycleDiagnostic(m *Model, parent int, members []int, ts map[[2]int]*Tie) Diagnostic {
	tr := m.Tree
	in := map[int]bool{}
	var ns, keys []string
	for _, mm := range members {
		in[mm] = true
		ns = append(ns, tr.Nodes[mm].Name)
		keys = append(keys, tr.display(mm))
	}
	sort.Strings(ns)
	sort.Strings(keys)
	var inside []namedTie
	for k, t := range ts {
		if in[k[0]] && in[k[1]] {
			inside = append(inside, namedTie{tr.Nodes[k[0]].Name, tr.Nodes[k[1]].Name, t})
		}
	}
	loc, notes, pic := circleReport(m, "uses", "module", len(members), inside)
	return Diagnostic{
		Lint:     "module-cycle",
		Key:      strings.Join(keys, ","),
		Message:  fmt.Sprintf("%s in `%s` use each other in a circle", counted(len(members), "module"), tr.display(parent)),
		Location: loc,
		Notes:    append([]string{"members: " + listed(ns)}, notes...),
		Picture:  pic,
	}
}

// ---- tangle ----

func fullName(m *Model, g *Graph, thing int) string {
	t := g.Things[thing]
	return m.Tree.display(t.Home) + "::" + m.Items[t.Item].Name
}

func tangles(m *Model, g *Graph) []Diagnostic {
	type written struct {
		modules map[int]bool
		ties    map[[2]int]*Tie
	}
	ws := make([]written, len(g.Tangles))
	for i, members := range g.Tangles {
		ws[i] = written{modules: map[int]bool{}, ties: map[[2]int]*Tie{}}
		for _, mm := range members {
			ws[i].modules[g.Things[mm].Home] = true
		}
	}
	for _, u := range g.Uses {
		if u.User < 0 {
			continue
		}
		used := g.ThingOf[u.Item]
		t := g.TangleOf[u.User]
		if used < 0 || t < 0 || u.User == used || g.TangleOf[used] != t {
			continue
		}
		r := m.References[u.Reference]
		ws[t].modules[r.From] = true
		k := [2]int{u.User, used}
		if ws[t].ties[k] == nil {
			ws[t].ties[k] = newTie()
		}
		ws[t].ties[k].add(u.Item, r, r.From)
	}
	var out []Diagnostic
	for i, w := range ws {
		members := g.Tangles[i]
		if !(len(w.modules) > 1 || len(members) > HeldInMind) {
			continue
		}
		var ns []string
		for _, mm := range members {
			ns = append(ns, fullName(m, g, mm))
		}
		sort.Strings(ns)
		var ties []namedTie
		for k, t := range w.ties {
			ties = append(ties, namedTie{fullName(m, g, k[0]), fullName(m, g, k[1]), t})
		}
		loc, notes, pic := circleReport(m, "knows", "thing", len(members), ties)
		all := []string{"members: " + listed(ns)}
		var mods []string
		for mod := range w.modules {
			mods = append(mods, m.Tree.display(mod))
		}
		sort.Strings(mods)
		if len(mods) > 1 {
			all = append(all, "written in: "+listed(mods))
		}
		msg := fmt.Sprintf("%d things know each other in a circle, across %s", len(ns), counted(len(mods), "module"))
		if len(mods) == 1 {
			msg = fmt.Sprintf("%d things know each other in a circle, all in `%s`", len(ns), mods[0])
		}
		out = append(out, Diagnostic{Lint: "tangle", Key: strings.Join(ns, ","), Message: msg, Location: loc,
			Notes: append(all, notes...), Picture: pic})
	}
	return out
}

// ---- field-leak ----

func bareName(m *Model, f int) string {
	n := m.Items[f].Name
	return n[strings.LastIndex(n, "::")+2:]
}

// fieldLeaks: a struct guards its fields when it keeps at least one of them
// to itself and has a method of its own, not a wrapper, that uses them.
// Then each field that code outside its package writes, takes the address
// of, indexes, ranges over or calls a method on, directly or through a
// getter or setter, is known outside it. Reading a value is no leak.
func fieldLeaks(m *Model, g *Graph) []Diagnostic {
	fields := map[int][]int{}
	var owners []int
	for i, it := range m.Items {
		if it.Kind != KField || it.Empty || it.Parent < 0 || g.ThingOf[i] < 0 {
			continue
		}
		if m.Items[it.Parent].Kind != KType {
			continue
		}
		t := g.ThingOf[i]
		if fields[t] == nil {
			owners = append(owners, t)
		}
		fields[t] = append(fields[t], i)
	}
	outside := map[int][]Use{}
	working := map[int]bool{}
	for _, u := range g.Uses {
		if m.Items[u.Item].Kind != KField {
			continue
		}
		owner := g.ThingOf[u.Item]
		if owner < 0 {
			continue
		}
		home := g.Things[owner].Home
		insider := false
		if u.User >= 0 {
			insider = g.Things[u.User].Home == home ||
				(g.TangleOf[owner] >= 0 && g.TangleOf[u.User] == g.TangleOf[owner])
		} else {
			insider = m.References[u.Reference].From == home
		}
		if !insider {
			outside[u.Item] = append(outside[u.Item], u)
		} else if u.User == owner {
			if h := m.References[u.Reference].Holder; h >= 0 && m.Items[h].Kind == KFunction && m.Items[h].TakesSelf {
				working[owner] = true
			}
		}
	}
	sort.Ints(owners)
	var out []Diagnostic
	for _, thing := range owners {
		fs := fields[thing]
		kept := false
		for _, f := range fs {
			if _, ok := outside[f]; !ok {
				kept = true
			}
		}
		if !kept || !working[thing] {
			continue
		}
		for _, f := range fs {
			var uses []Use
			for _, u := range outside[f] {
				if !u.Copies {
					uses = append(uses, u)
				}
			}
			if len(uses) == 0 {
				continue
			}
			var known, keptNames []string
			for _, o := range fs {
				if o == f {
					continue
				}
				us, ok := outside[o]
				if !ok {
					keptNames = append(keptNames, bareName(m, o))
					continue
				}
				for _, u := range us {
					if !u.Copies {
						known = append(known, bareName(m, o))
						break
					}
				}
			}
			out = append(out, leakDiagnostic(m, g, thing, f, uses, known, keptNames))
		}
	}
	return out
}

func leakDiagnostic(m *Model, g *Graph, thing, field int, uses []Use, known, kept []string) Diagnostic {
	tr := m.Tree
	sites := map[Location]string{}
	places := map[int]map[Location]bool{}
	for _, u := range uses {
		r := m.References[u.Reference]
		user := tr.display(r.From)
		if r.Holder >= 0 {
			user = tr.display(m.Items[r.Holder].Owner) + "::" + m.Items[r.Holder].Name
		}
		if u.Through >= 0 {
			user += "`, through `" + m.Items[u.Through].Name
		}
		at := Location{r.File, r.Line}
		if places[r.From] == nil {
			places[r.From] = map[Location]bool{}
		}
		places[r.From][at] = true
		if _, ok := sites[at]; !ok {
			sites[at] = "`" + user + "`"
		}
	}
	var locs []Location
	for l := range sites {
		locs = append(locs, l)
	}
	sort.Slice(locs, func(i, j int) bool { return less(locs[i], locs[j]) })
	var notes []string
	for i, l := range locs {
		if i == noteLimit {
			notes = append(notes, "and "+counted(len(locs)-noteLimit, "more place"))
			break
		}
		notes = append(notes, l.String()+": "+sites[l])
	}
	type mc struct {
		n    int
		name string
	}
	var mods []mc
	for mod, ps := range places {
		mods = append(mods, mc{len(ps), tr.display(mod)})
	}
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].n != mods[j].n {
			return mods[i].n > mods[j].n
		}
		return mods[i].name < mods[j].name
	})
	moduleCount := len(mods)
	if len(mods) > HeldInMind {
		more := mods[HeldInMind:]
		sum := 0
		for _, x := range more {
			sum += x.n
		}
		mods = append(mods[:HeldInMind], mc{sum, "in " + counted(len(more), "more module")})
	}
	owner := g.Things[thing]
	typeName := m.Items[owner.Item].Name
	fname := bareName(m, field)
	pic := []string{"  " + tr.display(owner.Home) + "::" + typeName}
	for i, x := range mods {
		lead := "    " + fname + " ◀━┳━ "
		if i > 0 {
			lead = strings.Repeat(" ", len([]rune(fname))+4) + "    ┣━ "
		}
		if i == len(mods)-1 && i > 0 {
			lead = strings.Repeat(" ", len([]rune(fname))+4) + "    ┗━ "
		}
		pic = append(pic, fmt.Sprintf("%s%d  %s", lead, x.n, x.name))
	}
	if len(known) > 0 {
		pic = append(pic, "    also known outside: "+listed(known))
	}
	if len(kept) > 0 {
		pic = append(pic, "    kept: "+listed(kept))
	}
	return Diagnostic{
		Lint:     "field-leak",
		Key:      tr.display(owner.Home) + "::" + m.Items[field].Name,
		Message:  fmt.Sprintf("`%s` is known outside `%s`: %s in %s", m.Items[field].Name, typeName, counted(len(locs), "place"), counted(moduleCount, "module")),
		Location: &locs[0],
		Notes:    notes,
		Picture:  pic,
	}
}

// ---- shallow-module ----

func shallow(m *Model, g *Graph) []Diagnostic {
	tr := m.Tree
	judged := map[int]bool{}
	hasChild := map[int]bool{}
	for id, n := range tr.Nodes {
		if n.Parent >= 0 && !n.Test {
			hasChild[n.Parent] = true
		}
		_ = id
	}
	for id, n := range tr.Nodes {
		if n.Test {
			continue
		}
		if n.Kind == Package || (n.Kind == Module && hasChild[id]) {
			judged[id] = true
		}
	}
	home := func(item int) int {
		if t := g.ThingOf[item]; t >= 0 {
			return g.Things[t].Home
		}
		return -1
	}
	working := func(item int) bool {
		it := m.Items[item]
		_, w := m.Wrappers[item]
		return it.Kind == KFunction && !it.TraitImpl && !w
	}
	around := func(n int) []int {
		var out []int
		for _, a := range tr.ancestors(n) {
			if tr.Nodes[a].Kind == Workspace {
				break
			}
			out = append(out, a)
		}
		return out
	}
	total := map[int]int{}
	for i := range m.Items {
		h := home(i)
		if h < 0 || !working(i) {
			continue
		}
		for _, mod := range around(h) {
			if judged[mod] {
				total[mod]++
			}
		}
	}
	shown := map[int]map[int]bool{}
	for _, u := range g.Uses {
		at := home(u.Item)
		if at < 0 || !working(u.Item) {
			continue
		}
		r := m.References[u.Reference]
		user := r.From
		if u.User >= 0 {
			user = g.Things[u.User].Home
		}
		for _, mod := range around(at) {
			if tr.isWithin(user, mod) {
				break
			}
			if judged[mod] {
				if shown[mod] == nil {
					shown[mod] = map[int]bool{}
				}
				shown[mod][u.Item] = true
			}
		}
	}
	// The types each free function knows.
	known := map[int]map[int]bool{}
	for _, u := range g.Uses {
		used := g.ThingOf[u.Item]
		if u.User < 0 || used < 0 {
			continue
		}
		if m.Items[g.Things[u.User].Item].Kind == KFunction && m.Items[g.Things[used].Item].Kind == KType {
			if known[u.User] == nil {
				known[u.User] = map[int]bool{}
			}
			known[u.User][used] = true
		}
	}
	var mods []int
	for mod := range judged {
		mods = append(mods, mod)
	}
	sort.Ints(mods)
	var out []Diagnostic
	for _, mod := range mods {
		sh := shown[mod]
		if sh == nil {
			continue
		}
		hidden := total[mod] - len(sh)
		if hidden < 0 {
			hidden = 0
		}
		lists := shownLists(m, g, known, mod, sh)
		notes, long := tooLong(lists)
		if len(sh) > HeldInMind && len(notes) > 0 && hidden <= len(sh)*BreakEven {
			name := tr.display(mod)
			pic := []string{fmt.Sprintf("  shows %d ━━ hides %d", len(sh), hidden)}
			for _, l := range long {
				pic = append(pic, fmt.Sprintf("    %s: %d, more than %d", l.name, l.n, HeldInMind))
			}
			var loc *Location
			if f := firstFile(m, mod); f != "" {
				loc = &Location{f, 1}
			}
			out = append(out, Diagnostic{Lint: "shallow-module", Key: name,
				Message:  fmt.Sprintf("`%s` is shallow: it shows %d of its %s", name, len(sh), counted(total[mod], "function")),
				Location: loc, Notes: notes, Picture: pic})
		}
	}
	return out
}

func firstFile(m *Model, mod int) string {
	best := ""
	for _, it := range m.Items {
		if it.Owner == mod && (best == "" || it.File < best) {
			best = it.File
		}
	}
	return best
}

type namedList struct {
	name  string
	funcs []string
}

func shownLists(m *Model, g *Graph, known map[int]map[int]bool, mod int, shown map[int]bool) []namedList {
	lists := map[int][]string{}
	for f := range shown {
		thing := g.ThingOf[f]
		if thing < 0 {
			continue
		}
		on := []int{thing}
		if g.Things[thing].Item == f {
			var types []int
			for ty := range known[thing] {
				if m.Tree.isWithin(g.Things[ty].Home, mod) {
					types = append(types, ty)
				}
			}
			if len(types) > 0 {
				on = types
			}
		}
		for _, l := range on {
			lists[l] = append(lists[l], m.Items[f].Name)
		}
	}
	var out []namedList
	for thing, fs := range lists {
		sort.Strings(fs)
		out = append(out, namedList{m.Items[g.Things[thing].Item].Name, fs})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].funcs) != len(out[j].funcs) {
			return len(out[i].funcs) > len(out[j].funcs)
		}
		return out[i].name < out[j].name
	})
	return out
}

type longList struct {
	name string
	n    int
}

func tooLong(lists []namedList) ([]string, []longList) {
	var notes []string
	var long []longList
	if len(lists) > HeldInMind {
		var ns []string
		for _, l := range lists {
			ns = append(ns, l.name)
		}
		notes = append(notes, fmt.Sprintf("shows %s: %s", counted(len(lists), "thing"), listed(ns)))
		long = append(long, longList{"things", len(lists)})
	}
	for _, l := range lists {
		if len(l.funcs) > HeldInMind {
			notes = append(notes, fmt.Sprintf("`%s` shows %s: %s", l.name, counted(len(l.funcs), "function"), listed(l.funcs)))
			long = append(long, longList{l.name, len(l.funcs)})
		}
	}
	return notes, long
}
