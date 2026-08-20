---
name: wasm-analysis
mode: reverse
summary: Read and deobfuscate WebAssembly modules from web and edge applications.
---

# WebAssembly analysis

WebAssembly is a compilation target, so a `.wasm` file is closer to a stripped native binary than to JavaScript. But its structure is regular, its type system is explicit, and its host interface is narrow — all of which help.

## Structure

A module is a sequence of typed sections: types, imports, functions, tables, memory, globals, exports, code, and data. The section layout alone tells you a lot before any disassembly:

- **Imports** define everything the module can reach outside itself. This is the complete list of host capabilities — there is no ambient access, so the import list bounds what the module can possibly do.
- **Exports** define its API to the host.
- **Data** holds initialized memory: string literals and constant tables live here and are readable directly.
- **Name section**, when present, carries function names. Release builds often strip it; when it survives, analysis is dramatically easier.

## Identify the source language

The toolchain leaves clear traces:

- **Emscripten (C/C++):** a companion `.js` glue file, `_emscripten_` imports, `__cxa_` exception symbols.
- **Rust:** `_ZN` mangled names when unstripped, panic machinery, `wasm-bindgen` glue with distinctive shim exports.
- **Go:** a very large module, a `.js` runtime shim, and Go runtime symbol patterns.
- **AssemblyScript:** a small module with straightforward structure and its own runtime helpers.

The language determines memory layout conventions and what the glue code does, so establish it early.

## Reading the code

Convert to the text format for readability. Decompilers that lift Wasm to C-like pseudocode exist and help considerably, since raw Wasm is a stack machine and stack-machine code is tedious to follow by hand.

Key mechanics:

- **Linear memory** is one flat byte array. Pointers are offsets into it, and all structure is implicit — the same offset-arithmetic reading as native reversing.
- **Indirect calls** go through a function table by index, which is how C++ virtual dispatch and function pointers appear. Resolving them means tracing the index.
- **No registers or stack frames** in the native sense; locals are numbered. Variable identity comes from local indices.

## The glue code matters

For Emscripten and wasm-bindgen builds, the JavaScript glue implements the imports and marshals values across the boundary. Reading it tells you the calling conventions, how strings and objects are passed, and which host APIs are reachable. Often the glue answers your question without touching the Wasm at all.

Apply the `js-deobfuscation` skill to the glue if it is minified.

## Dynamic analysis

Browser devtools debug Wasm directly: breakpoints, stepping, and memory inspection. Since linear memory is a single `ArrayBuffer` accessible from JavaScript, you can read and modify the module's entire state from the console — an unusually strong position compared to native targets.

Instrumenting the import functions logs every host interaction, which for most modules is the interesting part.

## What this is usually for

Wasm shows up in three places worth reversing: obfuscated business logic moved out of JavaScript to make copying harder, client-side crypto and licensing checks, and performance-critical game or media code. In the first two cases the goal is usually recovering an algorithm, and the data section plus the import list gets you most of the way.
