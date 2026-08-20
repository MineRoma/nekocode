---
name: ghidra-workflow
mode: reverse
summary: Decompiler-driven analysis: navigate, retype, and annotate until pseudocode reads like source.
---

# Ghidra workflow

The decompiler's first output is a draft. Analysis is the process of feeding it type information until it becomes readable.

## Establish a foothold

Do not start at `main` in a stripped binary — you may not have one. Start where the binary is forced to be honest:

- **Imports.** Cross-reference interesting API calls backwards. `CreateFileW`, `send`, `RegSetValueEx` each anchor a behavior.
- **Strings.** Cross-reference a string to its user. Error messages are especially good: they name the function's purpose in the author's own words.
- **Entry points.** TLS callbacks and static initializers run before `main` and are a common hiding place.
- **Exports.** For a DLL, the export table is the intended API.

## The retyping loop

Pseudocode quality is a direct function of type accuracy. Iterate:

1. Find the function's real signature. Calling conventions and argument counts come from the call sites.
2. Replace `undefined4`/`void*` with real types. Every corrected type propagates outward.
3. Define structures when you see repeated `*(int *)(param_1 + 0x18)` patterns. Create the struct, assign it, and the offsets become field names.
4. Rename as you learn. `FUN_00401a20` → `parse_config_header`. Names are your memory across sessions.
5. Re-read the decompilation. It will have changed shape.

Repeat until the function reads like code someone wrote. If it does not, you have a wrong type somewhere.

## Structure recovery

The strongest signal is consistent offset arithmetic on the same pointer. Collect every offset touched, note the access width, and build the struct incrementally. Vtable pointers at offset 0 mean C++; follow the vtable to recover the class's methods as a group.

## Scripting

Repetitive work belongs in a script rather than in your hands. Typical wins: bulk-renaming functions from a recovered symbol table, applying a struct across many call sites, decrypting an obfuscated string table and patching the plaintext back in as comments, and tagging every function that reaches a given API.

Ghidra's headless mode makes this repeatable and reviewable. Prefer a committed script over manual clicks you cannot reproduce.

## Annotate for your future self

Comment intent, not mechanics. `// XOR key derived from build timestamp, see 0x401200` is useful. `// xor eax, eax` is not — that is already on screen.

Export a summary when you finish: entry points, recovered structures, the call graph of interesting paths, and open questions. The next session starts from that summary, not from a blank project.

## Cross-checking

The decompiler can be wrong, especially around obfuscated control flow, hand-written assembly, and jump tables. When pseudocode looks impossible, read the disassembly for that block. Trust the instructions over the C.
