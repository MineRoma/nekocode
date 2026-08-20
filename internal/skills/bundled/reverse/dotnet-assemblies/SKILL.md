---
name: dotnet-assemblies
mode: reverse
summary: Decompile, deobfuscate, and inspect .NET and Unity managed assemblies.
---

# .NET assemblies

Managed .NET is the easiest mainstream target: IL carries full metadata, so decompilation approaches the original source rather than approximating it. The work is usually deobfuscation, not decompilation.

## Confirming it is .NET

A CLR header in the PE, an `mscoree.dll` import in older binaries, `#Blob`/`#Strings`/`#US` metadata streams, and a `.text` section dominated by IL rather than machine code. Single-file published .NET Core apps bundle a native host — the managed assemblies are embedded resources, extract them first.

## Decompilation

IL preserves type names, method names, parameter names, and often source line info. A decompiler reconstructs near-source C#. If output looks like garbage, the assembly is obfuscated, not merely compiled.

## Recognizing obfuscation

- Identifiers replaced by unprintable Unicode, single letters, or repeated homoglyphs.
- Control flow flattened into a `switch` inside a `while(true)` with a state integer.
- All string literals replaced by calls to a decryption method taking an integer token.
- Proxy methods: every call goes through a generated wrapper.
- Anti-tamper: the module's static constructor decrypts method bodies at runtime.
- Fake or invalid metadata designed to crash specific decompilers.

## Deobfuscation order

1. **Unpack anti-tamper first.** If method bodies are encrypted at rest, everything else is blocked. Load the assembly in a controlled process, let the module initializer run, then dump from memory.
2. **Decrypt strings.** Locate the decryptor, run it over every token statically, and rewrite the call sites with literals. This alone often makes the code readable.
3. **Flatten proxy calls.** Replace wrapper calls with their targets.
4. **Restore control flow.** Undo the switch-dispatcher transformation.
5. **Rename.** Deterministic renaming makes the result diffable and reviewable, even if the original names are unrecoverable.

Known obfuscators (ConfuserEx, .NET Reactor, Babel, Agile.NET) have known unpackers. Identify the obfuscator by its artifacts before writing anything custom — the version matters.

## Dynamic inspection

Managed debuggers attach to a running process and let you set breakpoints on decompiled code, edit values, and call methods interactively. For an obfuscated sample this is often faster than static work: breakpoint the string decryptor and read the plaintext as it flows.

Runtime patching via reflection or IL rewriting lets you neuter a check without a full static recovery.

## Unity and IL2CPP

Unity games ship one of two ways. **Mono** builds keep managed `Assembly-CSharp.dll` and decompile normally. **IL2CPP** builds transpile IL to C++ and compile natively — see the `unity-il2cpp` skill, since the approach is entirely different.

## Practical order

1. Confirm managed, extract from single-file bundle if needed.
2. Identify the obfuscator from its fingerprints.
3. Defeat anti-tamper, then strings, then control flow.
4. Decompile and read.
5. Attach a debugger for anything still unclear.
