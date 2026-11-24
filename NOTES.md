# deslop-go: deslop-please's four lints, ported to Go

Build: `go build -o deslop-go .`; run: `./deslop-go [-C dir] [-lint a,b] [-counts] [-all] [patterns]` (default `./...`).
Same lint names, thresholds (HELD_IN_MIND = 7, BREAK_EVEN = 1) and report shape (message, `-->` location, notes,
picture). Built on go/packages + go/types instead of a rust-analyzer SCIP index. Fixtures in `testdata/`
mirror upstream's unit tests.

## Mapping

| deslop-please (Rust) | deslop-go |
| --- | --- |
| crate | the Go module |
| module tree | the module's directory tree; a package is the module of its directory; a directory without Go files is a module with no items |
| thing: type + fields + variants + methods / free fn / const or static | named type + fields + methods + typed consts of that type / package-level func / package-level var or const |
| test code (`#[cfg(test)]`, tests/) | `_test.go` files (not loaded) and `*test` helper packages (e.g. `pgtest`) |
| trait impl method (`impl Trait for T`) | a method that implements a method of some interface (ours or a dependency's, plus `error`) |
| wrapper: forwarder | a func whose single statement calls one of our funcs with its own params, on a param or a field path of the receiver |
| wrapper: accessor (getter/setter) | a method whose single statement returns receiver fields (`&r.f` or a pointer/slice/map/chan/func/interface field: lent; else copied) or assigns params to receiver fields (written) |
| `Access::Copy / Borrow / Call / Reach` | read value, arg, `T{F: v}`, `(r.f)()` / `range r.f`, a lent getter's result taken as is / method call, `len`, `cap`, `append(r.f, ..)` / write, `++`, index, slice, `&r.f`, `delete`, `clear`, `copy` into |
| `Plain::Scalar / Data / Nothing` | numbers, bool / string, slices, arrays and pointers of plain / `struct{}` |

## Where Go differs, and what the port does about it

- **Import cycles are illegal in Go**, so module-cycle can only come from the directory tree: a package and its
  sub-packages count as one module (Rust's module subtree), so `a/x → b` with `b/y → a` is a cycle between `a` and `b`.
  Tangles across packages are likewise impossible; a tangle is always one package, reported only when over 7 things.
- **field-leak's outsiders are other packages**: Go's encapsulation unit is the package, so
  code in the same package is an insider. Upstream counts any code but the type's own methods (and its tangle) as an
  outsider, free functions in the same module included.
- **A keyed struct literal is Copy** (upstream: a field named in a struct literal is given whole). In Rust a struct
  that keeps a private field cannot be built by literal outside its module; in Go `&T{F: v}` with unexported fields
  beside is the named-argument constructor.
- **A value-receiver method call on a field is Copy** (Go copies the receiver: Go's `.clone()`); upstream counts
  any `&self` method call as Call, a leak unless the field is a number.
- **`len`/`cap` are Call** (Rust's `.len()`), **`range` is Borrow** (Rust's `for x in &v`).
- **Typed constants are their type's variants** (Go's enum idiom; Rust's enum variants are part of the enum).
- **Type aliases are the type they name**; a type parameter returned by a getter is copied, not lent.
- Not ported: `wide-directory` and `long-file`; the allow file; generics are
  resolved to their origin.
