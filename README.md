# deslop-go

A Go port of the four structural lints of
[**deslop-please**](https://github.com/deterministic-machines/deslop-please) by
[@XiangpengHao](https://github.com/XiangpengHao), which this project takes as its inspiration and its spec: an
opinionated linter that follows *A Philosophy of Software Design*, **deep** modules and **narrow** interfaces.
deslop-please is Rust-only (Cargo and rust-analyzer's SCIP index); deslop-go reads a Go module with
`golang.org/x/tools/go/packages` and `go/types`, and keeps the original's lint names, thresholds and report shape.
All credit for the ideas goes to the original; the mistakes in translating them are mine.

```bash
go install github.com/Bombatomica64/deslop-go@latest
deslop-go                      # the module in the current directory, ./...
deslop-go -C path/to/module    # another module
deslop-go -lint field-leak     # one lint
deslop-go -counts              # only the counts by lint
deslop-go -all                 # every tie and place in the notes, not only the first seven
```

```
error[field-leak]: `RunQueue::Items` is known outside `RunQueue`: 1 place in 1 module
  --> sched/sched.go:3:1
  = note: sched/sched.go:3:1: `sched::Admit`
    queue::RunQueue
      Items ◀━┳━ 1  sched
      also known outside: Spare
      kept: n

deslop-go: 2 errors (module-cycle 0, tangle 0, field-leak 2, shallow-module 0)
```

## The lints

- **module-cycle**: sibling packages (a package counts with its sub-packages) that use each other in a circle.
- **tangle**: things (a type with its fields, methods and typed constants, a function, a package-level var or
  const) that know each other in a circle, across packages or more than seven of them.
- **field-leak**: a struct that keeps a field to itself and has a method working on its fields, whose other
  fields code in other packages writes, indexes, takes the address of or calls a method on (directly or through a
  getter or setter). Reading a value is no leak.
- **shallow-module**: a package with sub-packages that shows more functions than a reader holds in mind (7), at
  least as many as it hides, and a list that long.

How to fix what they find, in the original's words: from first principles, not by mechanical moves that only hide
the finding. For cycles and tangles, decide which layer each package belongs to; for leaks and shallow modules, ask
why the fields or functions exist and hide them behind a narrow interface, holistically.

[NOTES.md](NOTES.md) maps Rust's concepts to Go's and lists every place the port's semantics differ from the
original. The layout lints (`wide-directory`, `long-file`) and the allow file are not ported.

## License

MIT, as deslop-please.
