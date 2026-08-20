---
name: jvm-bytecode
mode: reverse
summary: Decompile and deobfuscate Java, Kotlin, and Android-adjacent JVM bytecode.
---

# JVM bytecode

Java class files retain enough metadata that decompilation is usually near-source. The work is handling obfuscation and understanding what the compiler synthesized.

## Structure

A `.class` file is a constant pool plus fields, methods, and attributes. A `.jar` is a zip of them with a manifest naming the main class. Decompilers reconstruct readable Java; different decompilers fail on different constructs, so keep two on hand.

Debug attributes, when present, carry original parameter names and line numbers. Without them you get `arg0`, `arg1`.

## Compiler artifacts to recognize

- **Bridge and synthetic methods** from generics and covariant returns — not in the source.
- **Type erasure:** generic parameters vanish at runtime, leaving casts. `List<String>` becomes `List` plus `(String)` casts.
- **Lambdas** compile to `invokedynamic` with a synthetic method holding the body; a decompiler usually re-sugars them, but the underlying shape shows through in bytecode.
- **String concatenation** becomes `StringBuilder` chains or `makeConcatWithConstants`.
- **Switch on strings** becomes a two-stage hashCode-then-equals dispatch.
- **Inner classes** become separate class files with `$` names and synthetic accessors for outer-scope fields.
- **Enums** become classes with static instances and a `$VALUES` array.

## Kotlin specifics

Kotlin adds `@Metadata` annotations carrying the real Kotlin signatures — read them, since they recover nullability, default parameters, and property structure that the Java view loses. Null checks appear as `Intrinsics.checkNotNullParameter` calls at method entry. `data class` generates `equals`/`hashCode`/`toString`/`copy`. Coroutines compile to state machines implementing `Continuation`, with the suspension points as switch cases — recognizable but verbose.

## Obfuscation

ProGuard/R8 renames and prunes; commercial obfuscators additionally encrypt strings, flatten control flow, insert unreachable exception handlers, and split methods.

Handle in order: strings first (find the decryptor, evaluate it over the whole table), then control flow, then renaming. Anything referenced reflectively or from configuration cannot be renamed and gives you anchors — as do framework superclass and interface names.

Deliberately malformed class files that crash decompilers but load fine in the JVM are a known trick. Bytecode-level repair, or reading the raw bytecode, gets past it.

## Bytecode instrumentation

When decompilation is blocked, run it instead. A bytecode manipulation library rewrites classes at load time, so you can log every method entry with arguments, dump the actual strings after decryption, and trace the real control flow. An agent attached at JVM startup does this without touching the jar on disk.

Reflection makes private state accessible for inspection at runtime.

## Practical order

1. Unzip, read the manifest, find the main class or entry points.
2. Decompile; note whether output is obfuscated.
3. Read `@Metadata` if Kotlin.
4. Defeat string encryption, then control flow.
5. Instrument at runtime for anything still unclear.
