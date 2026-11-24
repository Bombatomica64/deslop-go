package main

// The model: the tree of modules (directories of the Go module, a package
// being the module of its directory), the items (types, functions, methods,
// fields, package-level vars and consts), the references between them, and
// the wrappers (getters, setters, forwarders). It mirrors deslop-please's
// src/model, built from go/types instead of a SCIP index.

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type NodeKind int

const (
	Workspace NodeKind = iota
	Package            // the Go module: deslop's crate
	Module             // a directory under it, a package or not
)

type Node struct {
	Kind     NodeKind
	Name     string // last path element
	Path     string // module-relative directory, "" for the crate
	Parent   int    // -1 for the workspace
	Test     bool   // a *test helper package, or under one
	HasFiles bool   // a Go package lives here
}

type Tree struct {
	Nodes []Node
	byDir map[string]int
	crate string
}

func newTree(crate string) *Tree {
	t := &Tree{byDir: map[string]int{}, crate: crate}
	t.Nodes = append(t.Nodes, Node{Kind: Workspace, Parent: -1})
	t.Nodes = append(t.Nodes, Node{Kind: Package, Name: crate, Parent: 0})
	t.byDir[""] = 1
	return t
}

func (t *Tree) node(dir string) int {
	if id, ok := t.byDir[dir]; ok {
		return id
	}
	parent := 1
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		parent = t.node(dir[:i])
	}
	name := dir[strings.LastIndex(dir, "/")+1:]
	test := t.Nodes[parent].Test || (strings.HasSuffix(name, "test") && name != "test")
	t.Nodes = append(t.Nodes, Node{Kind: Module, Name: name, Path: dir, Parent: parent, Test: test})
	id := len(t.Nodes) - 1
	t.byDir[dir] = id
	return id
}

func (t *Tree) display(id int) string {
	n := t.Nodes[id]
	if n.Kind == Package {
		return t.crate
	}
	return n.Path
}

// ancestors: id, its parent, ..., the workspace.
func (t *Tree) ancestors(id int) []int {
	var out []int
	for ; id >= 0; id = t.Nodes[id].Parent {
		out = append(out, id)
	}
	return out
}

func (t *Tree) isWithin(id, of int) bool {
	for ; id >= 0; id = t.Nodes[id].Parent {
		if id == of {
			return true
		}
	}
	return false
}

func (t *Tree) commonAncestor(a, b int) int {
	seen := map[int]bool{}
	for _, x := range t.ancestors(a) {
		seen[x] = true
	}
	for _, y := range t.ancestors(b) {
		if seen[y] {
			return y
		}
	}
	return 0
}

type ItemKind int

const (
	KType ItemKind = iota
	KFunction
	KConstant // package-level const or var
	KField
)

type Item struct {
	Obj       types.Object
	Kind      ItemKind
	Name      string // `Type::method`, `Type::field`, `func`
	Owner     int    // module node it is written in
	Parent    int    // item index of the type it belongs to, -1 for none
	File      string
	Line      int
	TakesSelf bool // a method
	TraitImpl bool // a method that implements an interface method
	Empty     bool // a field that holds nothing: `_` or a zero-size type
}

type Reference struct {
	Item   int
	From   int // module node the reference is written in
	Holder int // item whose code holds it, -1 for none
	File   string
	Line   int
	Access Access // what is done with the field, or the call's result
	Plain  bool   // nothing but its value is taken
}

type WrapperKind int

const (
	Forwarder WrapperKind = iota
	Accessor
)

type Wrapper struct {
	Kind    WrapperKind
	Target  int   // forwarder: the function it passes the call to
	Fields  []int // forwarder: fields of the receiver the call goes through
	Lent    []int // accessor: fields handed out by reference (pointer, slice, map, ...)
	Copied  []int // accessor: fields handed out by value
	Written []int // accessor: fields a setter overwrites
}

type Model struct {
	Tree       *Tree
	Items      []Item
	itemOf     map[types.Object]int
	References []Reference
	Wrappers   map[int]*Wrapper
	Fset       *token.FileSet
	root       string
}

func (m *Model) rel(pos token.Pos) (string, int) {
	p := m.Fset.Position(pos)
	r, err := filepath.Rel(m.root, p.Filename)
	if err != nil {
		r = p.Filename
	}
	return r, p.Line
}

func origin(obj types.Object) types.Object {
	switch o := obj.(type) {
	case *types.Func:
		return o.Origin()
	case *types.Var:
		return o.Origin()
	}
	return obj
}

func load(dir string, patterns []string) (*Model, []*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedModule,
		Dir:   dir,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, nil, err
	}
	var mod *packages.Module
	for _, p := range pkgs {
		for _, e := range p.Errors {
			os.Stderr.WriteString("warning: " + e.Error() + "\n")
		}
		if p.Module != nil {
			mod = p.Module
		}
	}
	root, _ := filepath.Abs(dir)
	crate := "crate"
	if mod != nil {
		root, crate = mod.Dir, mod.Path
	}
	m := &Model{Tree: newTree(crate), itemOf: map[types.Object]int{}, Wrappers: map[int]*Wrapper{}, root: root}
	if len(pkgs) > 0 {
		m.Fset = pkgs[0].Fset
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PkgPath < pkgs[j].PkgPath })
	ours := map[*types.Package]int{}
	for _, p := range pkgs {
		rel := strings.TrimPrefix(strings.TrimPrefix(p.PkgPath, crate), "/")
		id := m.Tree.node(rel)
		m.Tree.Nodes[id].HasFiles = true
		ours[p.Types] = id
	}
	b := &builder{m: m, ours: ours}
	for _, p := range pkgs {
		b.items(p)
	}
	for _, p := range pkgs {
		b.references(p)
	}
	for _, p := range pkgs {
		b.wrappers(p)
	}
	b.traitImpls(pkgs)
	return m, pkgs, nil
}

type builder struct {
	m    *Model
	ours map[*types.Package]int
}

func (b *builder) add(obj types.Object, kind ItemKind, name string, parent int) int {
	obj = origin(obj)
	if id, ok := b.m.itemOf[obj]; ok {
		return id
	}
	file, line := b.m.rel(obj.Pos())
	it := Item{Obj: obj, Kind: kind, Name: name, Owner: b.ours[obj.Pkg()], Parent: parent, File: file, Line: line}
	b.m.Items = append(b.m.Items, it)
	id := len(b.m.Items) - 1
	b.m.itemOf[obj] = id
	return id
}

// items: every package-level type (with its fields and methods), function,
// var and const.
func (b *builder) items(p *packages.Package) {
	scope := p.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		switch o := obj.(type) {
		case *types.TypeName:
			if o.IsAlias() {
				continue // an alias is the type it names
			}
			ty := b.add(o, KType, name, -1)
			b.fields(o.Type().Underlying(), ty, name)
			if n, ok := o.Type().(*types.Named); ok {
				for i := 0; i < n.NumMethods(); i++ {
					f := n.Method(i)
					id := b.add(f, KFunction, name+"::"+f.Name(), ty)
					b.m.Items[id].TakesSelf = true
				}
				if it, ok := n.Underlying().(*types.Interface); ok {
					for i := 0; i < it.NumExplicitMethods(); i++ {
						f := it.ExplicitMethod(i)
						b.add(f, KFunction, name+"::"+f.Name(), ty)
					}
				}
			}
		case *types.Func:
			b.add(o, KFunction, name, -1)
		case *types.Const:
			// A constant of a type of its package is a variant of that type,
			// as a Rust enum's are: Go's enum idiom.
			parent := -1
			if n, ok := types.Unalias(o.Type()).(*types.Named); ok && n.Obj().Pkg() == o.Pkg() {
				parent = b.add(n.Obj(), KType, n.Obj().Name(), -1)
			}
			if name != "_" {
				b.add(o, KConstant, name, parent)
			}
		case *types.Var:
			if name != "_" {
				b.add(o, KConstant, name, -1)
			}
		}
	}
}

// fields: the fields of a struct type, and of the anonymous struct types
// inside it, as parts of `ty`.
func (b *builder) fields(t types.Type, ty int, prefix string) {
	switch u := t.(type) {
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			id := b.add(f, KField, prefix+"::"+f.Name(), ty)
			if f.Name() == "_" || isEmpty(f.Type()) {
				b.m.Items[id].Empty = true
			}
			b.fields(f.Type(), ty, prefix+"::"+f.Name())
		}
	case *types.Pointer:
		b.fields(u.Elem(), ty, prefix)
	case *types.Slice:
		b.fields(u.Elem(), ty, prefix)
	case *types.Array:
		b.fields(u.Elem(), ty, prefix)
	case *types.Map:
		b.fields(u.Elem(), ty, prefix)
	}
}

func isEmpty(t types.Type) bool {
	s, ok := t.Underlying().(*types.Struct)
	return ok && s.NumFields() == 0
}

// declHolder: the item whose code a top-level declaration holds.
func (b *builder) declHolder(info *types.Info, decl ast.Decl, spec ast.Spec) int {
	var obj types.Object
	switch d := decl.(type) {
	case *ast.FuncDecl:
		obj = info.Defs[d.Name]
		if d.Name.Name == "init" && d.Recv == nil {
			// Each init is a function of its own; the type checker gives it no
			// scope entry.
			id := b.add(obj, KFunction, "init", -1)
			return id
		}
	case *ast.GenDecl:
		switch s := spec.(type) {
		case *ast.TypeSpec:
			obj = info.Defs[s.Name]
		case *ast.ValueSpec:
			for _, n := range s.Names {
				if n.Name != "_" {
					obj = info.Defs[n]
					break
				}
			}
		}
	}
	if obj == nil {
		return -1
	}
	if id, ok := b.m.itemOf[origin(obj)]; ok {
		return id
	}
	return -1
}

func (b *builder) item(obj types.Object) (int, bool) {
	if obj == nil || obj.Pkg() == nil {
		return 0, false
	}
	if tn, ok := obj.(*types.TypeName); ok && tn.IsAlias() {
		if n, ok := types.Unalias(tn.Type()).(*types.Named); ok {
			obj = n.Origin().Obj()
		}
	}
	if obj.Pkg() == nil {
		return 0, false
	}
	if _, ok := b.ours[obj.Pkg()]; !ok {
		return 0, false
	}
	id, ok := b.m.itemOf[origin(obj)]
	return id, ok
}

func (b *builder) references(p *packages.Package) {
	from := b.ours[p.Types]
	info := p.TypesInfo
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			var specs []ast.Spec
			if g, ok := decl.(*ast.GenDecl); ok {
				specs = g.Specs
			} else {
				specs = []ast.Spec{nil}
			}
			for _, spec := range specs {
				holder := b.declHolder(info, decl, spec)
				var root ast.Node = decl
				if spec != nil {
					root = spec
				}
				b.walk(info, root, from, holder)
			}
		}
	}
}

// walk records each use of an item under `root`, with whether it only reads it.
func (b *builder) walk(info *types.Info, root ast.Node, from, holder int) {
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		defer func() { stack = append(stack, n) }()
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := info.Uses[id]
		if obj == nil {
			return true
		}
		item, ok := b.item(obj)
		if !ok {
			return true
		}
		// What the use is: the selector it names, or the identifier.
		var expr ast.Expr = id
		parents := stack
		if len(parents) > 0 {
			if sel, ok := parents[len(parents)-1].(*ast.SelectorExpr); ok && sel.Sel == id {
				expr = sel
				parents = parents[:len(parents)-1]
				// A promoted field or method passes through embedded fields.
				if s := info.Selections[sel]; s != nil && len(s.Index()) > 1 {
					a, _ := classify(info, sel, parents)
					b.embedded(s, from, holder, id.Pos(), a)
				}
			}
		}
		// A call of a function is classified by what is done with its result.
		if _, fn := obj.(*types.Func); fn && len(parents) > 0 {
			if call, ok := parents[len(parents)-1].(*ast.CallExpr); ok && ast.Unparen(call.Fun) == expr {
				expr = call
				parents = parents[:len(parents)-1]
			}
		}
		file, line := b.m.rel(id.Pos())
		a, plain := classify(info, expr, parents)
		b.m.References = append(b.m.References, Reference{
			Item: item, From: from, Holder: holder, File: file, Line: line,
			Access: a, Plain: plain,
		})
		return true
	})
}

// embedded adds the uses of the embedded fields a selection goes through.
func (b *builder) embedded(s *types.Selection, from, holder int, pos token.Pos, a Access) {
	t := s.Recv()
	idx := s.Index()
	for _, i := range idx[:len(idx)-1] {
		if p, ok := t.Underlying().(*types.Pointer); ok {
			t = p.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return
		}
		f := st.Field(i)
		if item, ok := b.item(f); ok {
			file, line := b.m.rel(pos)
			b.m.References = append(b.m.References, Reference{Item: item, From: from, Holder: holder, File: file, Line: line, Access: a})
		}
		t = f.Type()
	}
}

// Access: what code does with a field it names (deslop-please's
// source/uses.rs), or with what a getter call hands out.
type Access int

const (
	Copy   Access = iota // takes its value: `q.Head + 1`, `f(q.Head)`, `T{F: v}`, `(q.Now)()`
	Borrow               // reads it in place: `range q.Items`, a lent getter's result passed on
	Call                 // calls a method on it: `q.Items.Len()`, `len(q.Items)`, `append(q.Items, x)`
	Reach                // writes, indexes, slices or takes its address: `q.Head = 0`, `q.Items[0]`, `&q.Items`
)

// classify: what is done with `expr` under `parents` (innermost last), and
// whether nothing but its value is taken (then a field is copied and a lent
// getter's result borrowed, unless it is dereferenced or a field of it read).
func classify(info *types.Info, expr ast.Expr, parents []ast.Node) (Access, bool) {
	goesOn := false
	for i := len(parents) - 1; i >= 0; i-- {
		switch p := parents[i].(type) {
		case *ast.ParenExpr:
			expr = p
			continue
		case *ast.StarExpr:
			expr = p
			goesOn = true
			continue
		case *ast.SelectorExpr:
			if p.X != expr {
				return Copy, false
			}
			s := info.Selections[p]
			if s != nil && s.Kind() == types.FieldVal {
				expr = p // a part of it: what is done to the part is done to it
				goesOn = true
				continue
			}
			// A method with a value receiver is given a copy: Go's `.clone()`.
			if s != nil && s.Kind() == types.MethodVal {
				if sig, ok := s.Obj().Type().(*types.Signature); ok && sig.Recv() != nil {
					if _, ptr := sig.Recv().Type().(*types.Pointer); !ptr && !types.IsInterface(sig.Recv().Type()) {
						return Copy, false
					}
				}
			}
			return Call, false
		case *ast.IndexExpr:
			if p.X == expr {
				return Reach, false
			}
			return Copy, false
		case *ast.IndexListExpr:
			if p.X == expr {
				return Reach, false
			}
			return Copy, false
		case *ast.SliceExpr:
			if p.X == expr {
				return Reach, false
			}
			return Copy, false
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				return Reach, false // a Go pointer can write: no shared borrow
			}
			return Copy, false
		case *ast.AssignStmt:
			for _, l := range p.Lhs {
				if l == expr {
					return Reach, false
				}
			}
			return Copy, !goesOn
		case *ast.IncDecStmt:
			return Reach, false
		case *ast.RangeStmt:
			if p.X == expr {
				return Borrow, false
			}
			return Reach, false
		case *ast.CallExpr:
			if p.Fun == expr {
				return Copy, false // `(q.f)(x)`: the function value is read
			}
			if id, ok := ast.Unparen(p.Fun).(*ast.Ident); ok {
				if _, builtin := info.Uses[id].(*types.Builtin); builtin {
					first := len(p.Args) > 0 && p.Args[0] == expr
					switch id.Name {
					case "len", "cap":
						return Call, false
					case "append":
						if first {
							return Call, false
						}
					case "delete", "clear", "close":
						return Reach, false
					case "copy":
						if first {
							return Reach, false
						}
					}
				}
			}
			return Copy, !goesOn
		default:
			return Copy, !goesOn
		}
	}
	return Copy, !goesOn
}

// Plain: what a field's type holds (deslop-please's `Plain`).
type Plain int

const (
	Scalar  Plain = iota // a number or bool
	Data                 // text, or numbers and text in slices, arrays, pointers
	Nothing              // struct{}
	NotPlain
)

func plainOf(t types.Type) Plain {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Info()&types.IsString != 0 {
			return Data
		}
		if u.Info()&(types.IsNumeric|types.IsBoolean) != 0 {
			return Scalar
		}
	case *types.Slice:
		if plainOf(u.Elem()) != NotPlain {
			return Data
		}
	case *types.Array:
		if plainOf(u.Elem()) != NotPlain {
			return Data
		}
	case *types.Pointer:
		if plainOf(u.Elem()) != NotPlain {
			return Data
		}
	case *types.Struct:
		if u.NumFields() == 0 {
			return Nothing
		}
	}
	return NotPlain
}

// onlyReads: whether code that does `a` with a field of type `t` only reads
// its value: takes a copy, borrows plain data, or calls a method on a number.
func onlyReads(a Access, t types.Type) bool {
	switch a {
	case Copy:
		return true
	case Borrow:
		return plainOf(t) != NotPlain
	case Call:
		return plainOf(t) == Scalar
	}
	return false
}

// wrappers: the getters, setters and forwarders, told by their bodies.
func (b *builder) wrappers(p *packages.Package) {
	info := p.TypesInfo
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || len(fd.Body.List) != 1 {
				continue
			}
			fn, ok := b.item(info.Defs[fd.Name])
			if !ok {
				continue
			}
			if w := b.forwarder(info, fd); w != nil {
				if !b.reaches(w.Target, fn) {
					b.m.Wrappers[fn] = w
				}
				continue
			}
			if w := b.accessor(info, fd, fn); w != nil {
				b.m.Wrappers[fn] = w
			}
		}
	}
}

func (b *builder) reaches(from, to int) bool {
	for at, n := from, 0; n < 1000; n++ {
		if at == to {
			return true
		}
		w, ok := b.m.Wrappers[at]
		if !ok || w.Kind != Forwarder {
			return false
		}
		at = w.Target
	}
	return true
}

func params(info *types.Info, fd *ast.FuncDecl) (map[types.Object]bool, types.Object) {
	ps := map[types.Object]bool{}
	var recv types.Object
	if fd.Recv != nil {
		for _, f := range fd.Recv.List {
			for _, n := range f.Names {
				recv = info.Defs[n]
			}
		}
	}
	for _, f := range fd.Type.Params.List {
		for _, n := range f.Names {
			ps[info.Defs[n]] = true
		}
	}
	return ps, recv
}

// forwarder: a body that only calls one function of ours, passing its own
// parameters, on one of them or on a field path of the receiver.
func (b *builder) forwarder(info *types.Info, fd *ast.FuncDecl) *Wrapper {
	var call *ast.CallExpr
	switch s := fd.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 1 {
			call, _ = s.Results[0].(*ast.CallExpr)
		}
	case *ast.ExprStmt:
		call, _ = s.X.(*ast.CallExpr)
	}
	if call == nil {
		return nil
	}
	ps, recv := params(info, fd)
	for _, a := range call.Args {
		id, ok := a.(*ast.Ident)
		if !ok || !ps[info.Uses[id]] {
			return nil
		}
	}
	var fields []int
	var target types.Object
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		target = info.Uses[fun]
	case *ast.SelectorExpr:
		target = info.Uses[fun.Sel]
		if s := info.Selections[fun]; s != nil {
			// The receiver: a parameter, or a field path of the receiver.
			x := fun.X
			for {
				if sel, ok := x.(*ast.SelectorExpr); ok {
					fs := info.Selections[sel]
					if fs == nil || fs.Kind() != types.FieldVal {
						return nil
					}
					item, ok := b.item(fs.Obj())
					if !ok {
						return nil
					}
					fields = append(fields, item)
					x = sel.X
					continue
				}
				break
			}
			id, ok := x.(*ast.Ident)
			if !ok {
				return nil
			}
			o := info.Uses[id]
			if !(ps[o] || (recv != nil && o == recv)) {
				return nil
			}
			if o == recv && len(fields) == 0 {
				// `r.other(x)`: a method of its own type with the receiver.
			}
		}
	default:
		return nil
	}
	if _, ok := target.(*types.Func); !ok {
		return nil
	}
	t, ok := b.item(target)
	if !ok {
		return nil
	}
	return &Wrapper{Kind: Forwarder, Target: t, Fields: fields}
}

// accessor: a method whose body only hands out fields of its receiver, or
// overwrites one with a parameter, computing nothing.
func (b *builder) accessor(info *types.Info, fd *ast.FuncDecl, fn int) *Wrapper {
	if fd.Recv == nil {
		return nil
	}
	ps, recv := params(info, fd)
	if recv == nil {
		return nil
	}
	own := func(e ast.Expr) (int, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return 0, false
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || info.Uses[x] != recv {
			return 0, false
		}
		s := info.Selections[sel]
		if s == nil || s.Kind() != types.FieldVal || len(s.Index()) != 1 {
			return 0, false
		}
		item, ok := b.item(s.Obj())
		if !ok || b.m.Items[item].Parent != b.m.Items[fn].Parent {
			return 0, false
		}
		return item, true
	}
	w := &Wrapper{Kind: Accessor}
	switch s := fd.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 0 {
			return nil
		}
		for _, r := range s.Results {
			if u, ok := r.(*ast.UnaryExpr); ok && u.Op == token.AND {
				f, ok := own(u.X)
				if !ok {
					return nil
				}
				w.Lent = append(w.Lent, f)
				continue
			}
			f, ok := own(r)
			if !ok {
				return nil
			}
			if byReference(b.m.Items[f].Obj.Type()) {
				w.Lent = append(w.Lent, f)
			} else {
				w.Copied = append(w.Copied, f)
			}
		}
	case *ast.AssignStmt:
		if len(s.Lhs) != len(s.Rhs) || (s.Tok != token.ASSIGN) {
			return nil
		}
		for i, l := range s.Lhs {
			f, ok := own(l)
			if !ok {
				return nil
			}
			id, ok := s.Rhs[i].(*ast.Ident)
			if !ok || !ps[info.Uses[id]] {
				return nil
			}
			w.Written = append(w.Written, f)
		}
	default:
		return nil
	}
	return w
}

// byReference: a value of `t` shares what it holds when copied.
func byReference(t types.Type) bool {
	if _, ok := t.(*types.TypeParam); ok {
		return false // returned by value, as its instantiation is
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	}
	return false
}

// traitImpls marks the methods that implement a method of an interface,
// ours or a dependency's: Go's `impl Trait for Type` is implicit.
func (b *builder) traitImpls(pkgs []*packages.Package) {
	var ifaces []*types.Interface
	seen := map[*types.Package]bool{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Types == nil || seen[p.Types] {
			return
		}
		seen[p.Types] = true
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || !tn.Exported() && b.ours[p.Types] == 0 {
				continue
			}
			if n, ok := tn.Type().(*types.Named); ok && n.TypeParams() == nil {
				if it, ok := n.Underlying().(*types.Interface); ok && it.NumMethods() > 0 && !it.IsImplicit() {
					if it.IsMethodSet() {
						ifaces = append(ifaces, it)
					}
				}
			}
		}
	})
	// Also `error`, `fmt.Stringer`-like universe interfaces.
	ifaces = append(ifaces, types.Universe.Lookup("error").Type().Underlying().(*types.Interface))
	for i := range b.m.Items {
		it := &b.m.Items[i]
		if it.Kind != KType {
			continue
		}
		tn := it.Obj.(*types.TypeName)
		n, ok := tn.Type().(*types.Named)
		if !ok || n.NumMethods() == 0 || n.TypeParams() != nil {
			continue
		}
		if _, isIface := n.Underlying().(*types.Interface); isIface {
			continue
		}
		ptr := types.NewPointer(n)
		for _, iface := range ifaces {
			if !types.Implements(ptr, iface) {
				continue
			}
			for j := 0; j < iface.NumMethods(); j++ {
				name := iface.Method(j).Name()
				for k := 0; k < n.NumMethods(); k++ {
					if m := n.Method(k); m.Name() == name {
						if id, ok := b.m.itemOf[origin(m)]; ok {
							b.m.Items[id].TraitImpl = true
						}
					}
				}
			}
		}
	}
}
