---
name: rust-binaries
mode: reverse
summary: Read Rust executables through mangled names, monomorphization, and panic metadata.
---

# Rust binaries

Rust binaries are hostile to casual reading — heavy inlining, aggressive monomorphization, no runtime type metadata — but they leak an unusual amount through panic infrastructure.

## Confirming it is Rust

`_ZN` legacy mangling or `_R` v0 mangling, `core::panicking::panic`, `rust_begin_unwind`, `rust_eh_personality`, and source paths embedded in panic metadata. Those paths often include the crate registry directory, which names every dependency and its version.

## Panic metadata is the map

Every `unwrap`, array index, and integer overflow check embeds a `&'static Location` with file, line, and column. Collect them and you have reconstructed the source tree layout, the crate names, and roughly which function corresponds to which source location. Nothing else in a stripped binary gives you this.

## Demangle before reading

Mangled Rust names encode the full path including generic parameters: `_ZN4core3ptr13drop_in_place17h...` demangles to something readable. Do this in bulk before analysis. The generic parameters in a monomorphized name tell you the concrete types a function was instantiated with — free type information.

## Monomorphization

A generic function compiled for three types becomes three separate functions with near-identical bodies. Recognize the duplication so you analyze the pattern once rather than three times. Conversely, a function appearing once means it was used with one concrete type.

## Data layout

- `String` and `Vec<T>`: pointer, capacity, length (order depends on version; verify against a known allocation).
- `&str`: pointer plus length, not NUL-terminated.
- `Option<T>` and `Result<T, E>`: often niche-optimized to the same size as `T`, so there may be no discriminant field at all. `Option<&T>` is a nullable pointer.
- Enums: a discriminant plus a union payload, unless niche optimization eliminated the discriminant.
- Trait objects: a fat pointer — data pointer plus vtable pointer. The vtable's first entries are drop glue, size, and alignment, then the trait methods.

`#[repr(Rust)]` gives no field-order guarantee; the compiler reorders for packing. Do not assume declaration order.

## Ownership artifacts

`drop_in_place` calls mark scope ends and tell you which type owned what. `Arc`/`Rc` clone and drop pairs reveal shared ownership graphs. Match them to reconstruct object lifetimes.

## Error handling

Rust has no exceptions in the C++ sense, but panics unwind. `Result` propagation via `?` compiles to a branch on the discriminant plus an early return, which is a recognizable diamond shape once you have seen a few. Iterator chains inline into loops that look nothing like the source; identify them by the closure bodies rather than the structure.

## Practical order

1. Confirm Rust, extract dependency versions from embedded paths.
2. Demangle all symbols in bulk.
3. Harvest panic locations to rebuild the source map.
4. Identify monomorphized clusters and pick one representative each.
5. Recover struct layouts from allocation sites and drop glue.
6. Work outward from `main` or the exported API.
