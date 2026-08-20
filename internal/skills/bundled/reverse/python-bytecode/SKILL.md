---
name: python-bytecode
mode: reverse
summary: Extract and decompile Python bundles, .pyc files, and frozen executables.
---

# Python bytecode

Python "executables" are almost always an interpreter plus compressed modules. The work is extraction first, decompilation second.

## Identify the bundler

- **PyInstaller:** `MEIPASS` strings, a `PYZ` archive, `pyi-` prefixed names. Extract the archive, then fix up the `.pyc` headers — extracted files often lack the magic and timestamp bytes and must be reconstructed for the target version.
- **py2exe:** a `PYTHONSCRIPT` PE resource holding marshalled code objects.
- **cx_Freeze:** a `library.zip` alongside the executable.
- **Nuitka:** genuinely compiled to C, then to native code. There is no bytecode to recover — treat it as a C binary, though the generated code has recognizable patterns and the module structure survives in strings.
- **Bare `.pyc`:** a 4-byte magic identifying the exact Python version, then flags, timestamp, size, and the marshalled code object.

Get the version right before anything else. Bytecode is version-specific and a decompiler pointed at the wrong version produces confident nonsense.

## Decompilation

Modern decompilers handle most released bytecode versions, with quality degrading on the newest releases and on heavily optimized comprehensions. When one fails, try another — they fail on different constructs. When all fail, disassemble to bytecode and read that; Python bytecode is stack-based and readable with modest practice.

Partial failure is normal and acceptable: a decompiler that recovers 90% of a module and errors on one function still gave you the module.

## Common obstacles

**Stripped docstrings and names** reduce readability but not structure — bytecode keeps `co_varnames` and `co_names` unless deliberately mangled.

**Bytecode obfuscation** appears as inserted dead opcodes, opaque jumps, or a custom opcode remapping in a patched interpreter. A remapped interpreter is the serious case: you must recover the mapping from the interpreter's dispatch table before any decompiler will work.

**Encrypted bundles** (commercial protectors) decrypt in memory. Hooking the import machinery or dumping code objects from a live process bypasses the encryption entirely.

**C extensions** (`.pyd`/`.so`) are native code. Analyze them as native binaries; the Python-facing boundary is the module init function and its method table, which names every exposed function.

## Dynamic extraction

Often the cheapest path: run the application in a controlled environment and use the interpreter against itself. Import the target module and inspect it, walk `sys.modules`, dump `func.__code__.co_code`, or hook `marshal.loads` to capture every code object the bundle loads. This works even when static extraction is blocked, because the interpreter must eventually see plaintext bytecode.

## Practical order

1. Identify the bundler and the exact Python version.
2. Extract the archive; repair `.pyc` headers.
3. Locate the entry module — usually named after the original script.
4. Decompile, accepting partial results.
5. For failures, read the disassembly or extract dynamically.
6. Analyze native extensions separately.
