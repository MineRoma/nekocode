---
name: unity-il2cpp
mode: reverse
summary: Recover C# structure from IL2CPP-compiled Unity games.
---

# Unity IL2CPP

IL2CPP transpiles C# to C++ and compiles it natively, so there is no managed assembly to decompile. What survives is metadata — and it survives almost completely, which makes IL2CPP far more tractable than it first appears.

## The key file

`global-metadata.dat` contains type names, method names, field names, and signatures for the entire game. Paired with `libil2cpp.so` (Android) or `GameAssembly.dll` (desktop), a dumper reconstructs the full C# type tree with method addresses.

The output is effectively the game's API surface: every class, every method, every field offset. Generate it before anything else.

## Workflow

1. **Locate the pair.** Metadata under `assets/bin/Data/Managed/Metadata/`, the native library alongside it or in `lib/<abi>/`.
2. **Dump.** Produce type definitions plus method addresses. Identify the Unity version first (from `globalgamemanagers` or the metadata header) — the metadata format version determines which dumper works.
3. **Apply to the disassembler.** Import the recovered symbols so `sub_1234` becomes `PlayerController$$TakeDamage`. This is the step that turns the project from unreadable to readable.
4. **Read method bodies natively.** They are ordinary compiled C++ now, but with correct names and field offsets they map back to the original C# closely.

## Field access

Recovered field offsets let you interpret `*(float *)(this + 0x3C)` as `this->health`. Define structures from the dump rather than reconstructing them by hand; the dump already has the layout.

## Runtime metadata calls

IL2CPP inserts runtime helpers with stable, recognizable names: type initialization, generic instantiation, boxing, array bounds checks, and string literal loading. Learning to skip them removes most of the visual noise from decompiled output.

String literals load by metadata index through a helper, so a raw string search finds them in the metadata rather than in the code. Resolve the index to see which string a call site uses.

## When metadata is protected

Some titles encrypt or strip `global-metadata.dat`, or ship a modified IL2CPP with a changed metadata layout. Options in order of effort: dump the decrypted metadata from process memory after initialization, recover the custom layout by reversing the metadata loader, or analyze the native code without names.

Memory dumping is usually correct here — the runtime must decrypt to function.

## Mono builds

Unity games not built with IL2CPP ship `Assembly-CSharp.dll` as a normal managed assembly. Check for it first: if present, use the `dotnet-assemblies` skill instead, and the work is dramatically easier.

## Asset bundles

Game data lives in `.assets` and `.bundle` files with a documented serialized format. Extracting meshes, textures, audio, and — importantly — `MonoBehaviour` serialized fields often answers questions about game logic without reading any code, since designers put balance values in data.

## Authorization

Single-player modding, interoperability, security research, and studying anti-cheat as a defender are legitimate. Building cheats for live multiplayer games harms other players and typically violates the terms you agreed to; that is not the same activity as understanding how a game works.
