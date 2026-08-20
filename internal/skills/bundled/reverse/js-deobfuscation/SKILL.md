---
name: js-deobfuscation
mode: reverse
summary: Recover readable source from minified, bundled, and obfuscated JavaScript.
---

# JavaScript deobfuscation

JavaScript always ships as source, so recovery is about undoing transformations rather than lifting machine code. Work through the layers in order — attempting semantic analysis before unbundling wastes effort.

## Layers, outermost first

1. **Source maps.** Check for them before anything else. A `.map` file or `sourceMappingURL` comment reconstructs the original files with original names, ending the task immediately. Production builds often ship them by accident.
2. **Bundling.** Webpack, Rollup, and esbuild concatenate modules into one file with a runtime loader. Split it back into modules first; each module is small and readable on its own, and the dependency graph reveals the architecture. Webpack's module map preserves original paths surprisingly often.
3. **Minification.** Mechanical and reversible in effect if not in detail: rename `a`, `b`, `c` to meaningful names as you infer purpose, restore formatting, and expand the comma-operator and sequence-expression compression that minifiers produce.
4. **Obfuscation.** Deliberate and adversarial. Handle with the passes below.

## Obfuscation patterns

**String array rotation.** All literals move to one array, accessed through an index function, with the array rotated at load by a self-invoking shuffler. Recover by evaluating the rotation once, then replacing every accessor call with its literal. This is the single highest-value pass — most obfuscated code becomes legible immediately.

**Control-flow flattening.** Straight-line code becomes a `while` loop over a `switch` driven by a state variable, often with the order encoded in a string split on a delimiter. Recover the sequence, then re-emit the blocks in order.

**Bracket-notation calls.** `window["ale"+"rt"]` hides API names from grep. Constant-fold string concatenation and normalize member access back to dot notation.

**Dead code and opaque predicates.** Branches that never execute, guarded by conditions that are constant but not obviously so. Constant-fold, then prune.

**Debugger traps.** `setInterval` loops calling `debugger`, or timing checks that detect a paused execution. Strip them before dynamic work.

**Proxy functions.** Every operation routed through generated wrappers (`function h(a,b){return a+b}`). Inline them.

## Working method

Prefer AST transformation over regular expressions. Parse to an AST, apply passes, re-emit. A regex approach on obfuscated code breaks on the first nested string and produces silently wrong output. Existing tools (webcrack, wakaru, and similar) implement these passes and handle the common obfuscator presets.

Run passes iteratively. Each one exposes structure that lets the next do more: string recovery reveals API names, which reveals which branches matter, which makes dead-code elimination effective.

Verify behavior is preserved. Deobfuscation that changes semantics is worse than none — spot-check by running both versions against the same inputs where feasible.

## Dynamic approaches

The runtime has already deobfuscated the code for you. Breakpointing in a browser or Node inspector, reading the actual call arguments, and dumping the resolved string table takes minutes and sidesteps every static defense. For a single question — what does it send, and where — this is usually the right first move.

## Reading the result

Once readable, the useful outputs are typically: the network endpoints and their payload shapes, the client-side validation and what it does not check, the storage keys and formats, and the crypto usage with where the keys come from. Write these down rather than leaving them implicit in recovered code.
