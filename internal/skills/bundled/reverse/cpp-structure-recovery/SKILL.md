---
name: cpp-structure-recovery
mode: reverse
summary: Rebuild classes, vtables, and templates from compiled C++.
---

# C++ structure recovery

C++ compiles to code with far more recoverable structure than C, because the object model leaves artifacts: vtables, mangled names, and constructor patterns that outline every class in the binary.

## Demangle first

Mangled names encode the full signature: namespace, class, method, parameter types, and const-ness. Both major schemes (Itanium ABI `_ZN...`, MSVC `?name@@...`) are fully demanglable.

Even a "stripped" C++ binary usually retains mangled names for exported symbols, RTTI, and anything referenced across translation units. That is a free class inventory — do it before anything else.

## Vtables outline the classes

A class with virtual functions has a vtable: a contiguous array of function pointers, typically in read-only data. Finding vtables gives you classes for free.

- An object's first member (offset 0) is usually its vtable pointer. A constructor writing a data-section address into `*this` is the giveaway.
- The vtable's entries are the virtual methods in declaration order.
- Multiple inheritance produces multiple vtables per class and thunks that adjust `this` before dispatching.
- Virtual destructors typically occupy the first slot or two.

Cross-referencing a vtable address finds the constructor; cross-referencing the constructor finds every allocation site of that class.

## RTTI is a gift

When RTTI is enabled — it usually is — type descriptors contain the class name as a plain string, and the inheritance hierarchy is explicitly encoded. That means you can recover the actual class names and the full inheritance graph without inference.

Look for the type descriptor structures adjacent to vtables. A single RTTI walk can name hundreds of classes.

## Constructors, destructors, and object lifetime

Constructors are recognizable: they take `this` as the first argument, write a vtable pointer, initialize members, and return `this`. Reading one gives you the class's complete member layout including types, which is the fastest route to a correct struct definition.

Destructors appear in the vtable and are called at scope exit. Exception-handling metadata identifies the destructor calls on unwind paths, which reveals which objects a function owned.

## The `this` pointer

In the common calling conventions `this` arrives in a fixed register (`rcx` on Windows x64, `rdi` on Linux x64) or as an implicit first stack argument. Typing that parameter as a pointer to your recovered class is what turns `*(int *)(a1 + 0x24)` into `this->count` across the whole function — and it propagates to callers.

## Templates and inlining

Templates monomorphize: `std::vector<int>` and `std::vector<Foo>` become unrelated code with similar shape. Recognize the duplication so you analyze the pattern once. Mangled names carry the concrete template arguments, so a demangled name tells you the instantiated types directly.

Heavy inlining is the main obstacle in optimized C++. Small accessors vanish entirely, and standard-library containers expand into raw pointer arithmetic. Learning the shapes of common container operations — vector growth, map lookup, string small-buffer optimization — is what makes optimized C++ readable.

## Standard library recognition

`std::string` with small-string optimization has a distinctive layout with an inline buffer and a capacity flag. `std::vector` is three pointers (begin, end, capacity-end). `std::map` is a red-black tree with recognizable rotation helpers. `std::shared_ptr` is a pointer plus a control block with two reference counts.

Identifying these removes enormous amounts of apparent complexity, because most of what looks like intricate logic is container bookkeeping.

## Practical order

1. Demangle every symbol available.
2. Walk RTTI to recover class names and hierarchy.
3. Locate vtables; map them to classes.
4. Read constructors to recover member layouts.
5. Define structs and apply them; retype `this` in each method.
6. Identify container operations and mentally collapse them.
7. Analyze the actual logic, which is now a small fraction of the code.
