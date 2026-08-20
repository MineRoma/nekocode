---
name: golang-binaries
mode: reverse
summary: Recover types, symbols, and goroutine structure from Go executables.
---

# Go binaries

Go binaries are large, statically linked, and full of runtime metadata. That metadata is the whole opportunity: even a stripped Go binary carries more recoverable structure than a typical stripped C binary.

## Confirming it is Go

`go:buildid`, a `.gopclntab` section, `runtime.morestack_noctxt`, and string-heavy `runtime.` references. The build ID also encodes the module path and dependency versions when the binary was built with modules.

## Recover symbols first

Do not analyze functions before restoring names. `.gopclntab` (the pc-line table) maps addresses to function names and survives ordinary stripping. Tooling — GoReSym, redress, `IDAGolangHelper`, Ghidra's Go analyzers — parses it and renames thousands of functions in one pass. Analysis before this step wastes effort on `FUN_` names you would rename anyway.

The module list from the build info is a shortcut to intent: a binary importing an SSH library and a SOCKS proxy tells you most of what it does before you read a single function.

## Type recovery

Go's runtime type descriptors (`runtime._type`) describe every type the binary uses, including struct field names and interface method sets. Recovering them gives you real struct definitions rather than offset arithmetic. This is what makes Go reversing tractable.

## Calling convention

Go 1.17+ uses a register-based ABI; earlier versions passed everything on the stack. Get this wrong and every function signature is wrong. Check the version from the build info first. Multiple return values appear as consecutive stack slots or registers, and the pervasive `(value, error)` pattern is recognizable once you know the layout.

## Strings

Go strings are a pointer plus a length, not NUL-terminated. Standard string extraction produces one giant run-together blob. Use Go-aware extraction, or the length field, to split them correctly.

## Goroutines and channels

`go func()` compiles to `runtime.newproc` with the target function as an argument — that is how you find concurrent entry points. Channel operations call `runtime.chansend`/`runtime.chanrecv`, and `select` becomes `runtime.selectgo`. Deferred calls appear as `runtime.deferproc`/`runtime.deferreturn` pairs, which is where cleanup and error handling live.

## Reading the result

Interface method calls dispatch through an itab, so the concrete target is not always statically obvious; resolve it by finding where the interface value was constructed. Bounds checks and `runtime.panicIndex` calls are noise — learn to skip them. Map and slice operations route through runtime helpers with recognizable signatures.

## Practical order

1. Confirm Go, extract build info and module list.
2. Restore symbols from `.gopclntab`.
3. Recover types, apply struct definitions.
4. Fix the calling convention for the detected version.
5. Locate `main.main` and the `runtime.newproc` call sites.
6. Analyze outward from there.
