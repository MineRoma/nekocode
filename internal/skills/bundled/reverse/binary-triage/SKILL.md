---
name: binary-triage
mode: reverse
summary: First-pass identification of an unknown binary before deep analysis.
---

# Binary triage

Run this before opening a disassembler. Ten minutes here saves hours of reading the wrong thing.

## Order of work

1. **Identify the container.** `file`, magic bytes, and section names tell you PE/ELF/Mach-O, architecture, and bitness. A `.exe` may be a self-extractor, an installer, or a script bundled with an interpreter.
2. **Measure entropy.** Section entropy above ~7.2 means packed or encrypted. Compare the ratio of raw size to virtual size; a tiny raw section that expands hugely at runtime is an unpacking stub.
3. **Read the import table.** Imports are the cheapest behavioral summary you will get. A binary importing only `LoadLibrary`/`GetProcAddress` resolves everything dynamically and is hiding its real imports.
4. **Pull strings, twice.** ASCII and UTF-16LE. Then look for what is *absent*: a network tool with no URLs and no socket imports is packed.
5. **Detect the toolchain.** Runtime artifacts identify the language: `go:buildid` and long `runtime.` symbol lists for Go, `_ZN`/`core::panicking` for Rust, `.pdata` plus MSVC runtime strings for C++, `mscoree`/CLR header for .NET, `libil2cpp` for Unity.
6. **Only then disassemble.** By this point you know which specialized workflow applies.

## What to record

Keep a running notes file from the first minute. For each finding, write down the evidence and your confidence:

```
sha256          <hash>
format          PE32+ / x86-64 / GUI subsystem
toolchain        MSVC 19.x, statically linked (confidence: high, CRT strings)
packing          none (entropy 6.1 across .text, imports resolved)
network          WinHTTP imports + 3 hardcoded https URLs (verified)
persistence      suspected registry Run key (inferred from string only)
```

Separating verified from inferred matters more than volume. A confident wrong claim costs more than an honest gap.

## Toolchain markers reference

| Marker | Toolchain | Next skill |
|---|---|---|
| `go:buildid`, `runtime.morestack` | Go | `golang-binaries` |
| `core::panicking`, `_ZN` mangling | Rust | `rust-binaries` |
| CLR header, `mscoree.dll` | .NET | `dotnet-assemblies` |
| `libil2cpp`, `global-metadata.dat` | Unity IL2CPP | `unity-il2cpp` |
| `PyInstaller`, `MEIPASS`, `python3x.dll` | Python bundle | `python-bytecode` |
| `.asar`, `electron.exe` | Electron | `electron-apps` |
| `LuaJIT`, `\x1bLJ` header | LuaJIT | `luajit-bytecode` |
| High entropy + tiny import table | Packed | `anti-reversing` |
| `classes.dex`, `AndroidManifest.xml` | Android | `android-apk` |

## Handling packed samples

Do not fight a packer manually before confirming it is custom. Check for known packers first (UPX, Themida, VMProtect signatures). For UPX and similar, static unpacking usually works. For virtualizing protectors, plan on dynamic analysis: let the unpacking stub run and dump the image from memory at the original entry point.

## Safety

Treat every unknown binary as live malware. Analyze in a disposable VM with no network or a controlled sink, snapshot before execution, and never run a sample on the host you take notes on.
