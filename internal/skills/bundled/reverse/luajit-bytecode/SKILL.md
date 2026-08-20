---
name: luajit-bytecode
mode: reverse
summary: Decompile LuaJIT and standard Lua bytecode from games and embedded scripts.
---

# LuaJIT and Lua bytecode

Lua is embedded in games, network appliances, and plugin systems. Compiled Lua retains a great deal of structure, and decompilers recover close-to-source output when the version matches.

## Identify the flavor and version

The header distinguishes standard Lua (`\x1bLua` plus a version byte) from LuaJIT (`\x1bLJ` plus a version byte). This matters enormously: LuaJIT bytecode is a different instruction set from PUC-Rio Lua, and the two need different tools.

The header also records number format (integer vs float, 32- vs 64-bit) and endianness. A mismatch produces garbage constants.

Common versions in the wild: Lua 5.1 (still dominant in games), 5.3/5.4, and LuaJIT 2.0/2.1 — source-compatible with 5.1 but not bytecode-compatible.

## Decompilation

Version-matched decompilers produce readable Lua including local variable names when debug info is present. Stripped bytecode loses names but keeps structure, control flow, and all constants.

When a decompiler fails on one function it usually still handles the rest of the file. Read the failing function as bytecode — Lua's register-based instruction set is compact and learnable quickly.

## Reading the constant pool

Even without decompiling, the constant table lists every string literal, every global name accessed, and every numeric constant. For a game script that alone gives you the API surface: which engine functions it calls, which event names it registers, which configuration keys it reads.

## Where the scripts live

In games, compiled Lua is usually inside an archive or resource bundle rather than loose on disk. Look for the loader in the native binary — a call to `luaL_loadbuffer` or `lua_load` shows where bytecode comes from, and hooking it captures every chunk the game loads, including runtime-generated ones.

Custom `require` implementations and virtual filesystems are common; the loader tells you the real layout.

## Obfuscation and custom VMs

**Bytecode obfuscation** — dead instructions, opaque jumps, encrypted strings in the constant pool. Find the decryption routine, apply it, prune the dead paths.

**Modified interpreter** — the embedder patched the VM's opcode numbering or added instructions, so stock decompilers produce nonsense. Recover the mapping from the interpreter's dispatch table in the native binary, then remap before decompiling. This is the serious case and is worth confirming early, because it explains otherwise inexplicable decompiler output.

## Dynamic approach

If the host application will run your code — via a console, a plugin path, or an injected chunk — the Lua runtime becomes your analysis tool. `debug.getinfo` enumerates functions and their source, `string.dump` re-emits bytecode, and walking `_G` inventories the entire exposed API with real names. That is often faster and more accurate than static work.

## Practical order

1. Read the header; establish flavor, version, and number format.
2. Extract chunks from archives, or hook the loader.
3. Dump the constant pool for a fast API inventory.
4. Decompile with a version-matched tool.
5. If output is nonsense, check for a remapped interpreter before blaming the decompiler.
