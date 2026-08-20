# Reverse mode skills

Skills Neko registers automatically the first time you switch to Reverse mode. Each directory holds one `SKILL.md` describing an approach to understanding artifacts you did not write.

They are embedded in the `neko` binary, so registration needs no files on disk. `/skills` lists them; `/addskills` re-registers them if you removed one.

## Where to start

`binary-triage` identifies an unknown artifact and names the skill that applies to it. When you do not know what you are holding, start there.

`notes-and-reporting` applies to every task — recovered understanding that is not written down is lost.

## Contents

| Skill | Use when |
|---|---|
| `binary-triage` | First contact with an unknown binary. Start here. |
| `notes-and-reporting` | Structuring findings; separating verified from inferred. |
| `ghidra-workflow` | Decompiler-driven analysis of native code. |
| `cpp-structure-recovery` | Compiled C++ — vtables, RTTI, classes, templates. |
| `anti-reversing` | Packers, obfuscators, VM protectors, anti-debug checks. |
| `dynamic-instrumentation` | Static analysis has stalled; you need real values. |
| `symbolic-execution` | Solving for an input instead of reading the algorithm. |
| `crypto-identification` | Recognizing algorithms, modes, and custom obfuscation. |
| `golang-binaries` | Go executables — symbol and type recovery. |
| `rust-binaries` | Rust executables — mangling, monomorphization, panic metadata. |
| `dotnet-assemblies` | .NET/CLR assemblies and Unity Mono builds. |
| `jvm-bytecode` | Java and Kotlin class files and jars. |
| `python-bytecode` | PyInstaller bundles, `.pyc`, frozen executables. |
| `js-deobfuscation` | Minified, bundled, or obfuscated JavaScript. |
| `wasm-analysis` | WebAssembly modules. |
| `electron-apps` | Desktop apps built on Electron. |
| `android-apk` | Android applications, DEX, and JNI libraries. |
| `unity-il2cpp` | Unity games compiled through IL2CPP. |
| `luajit-bytecode` | Embedded Lua and LuaJIT bytecode. |
| `firmware-analysis` | Embedded images, bare-metal, and RTOS code. |
| `protocol-analysis` | Undocumented network or wire protocols. |
| `file-format-analysis` | Undocumented file formats, archives, save data. |
| `api-reverse-engineering` | Private HTTP and RPC APIs. |
| `malware-static` | Characterizing a suspicious sample; extracting IOCs. |
| `ctf-reversing` | Time-scored challenges with one intended solution. |
| `legacy-code-archaeology` | Undocumented source you inherited, not a binary. |

## Scope

These describe method rather than specific tool invocations, so they stay useful as tooling changes.

They assume you have authorization for the target: your own software, an engagement you are hired for, a CTF, a malware sample, or interoperability work. Several include a note on where that line falls in their domain.
