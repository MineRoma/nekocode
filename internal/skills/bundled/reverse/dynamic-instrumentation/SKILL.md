---
name: dynamic-instrumentation
mode: reverse
summary: Hook and trace running processes when static analysis stalls.
---

# Dynamic instrumentation

Static analysis tells you what code *can* do. Instrumentation tells you what it *does*, with real arguments, on the path actually taken. When a target is packed, obfuscated, or simply large, hooking is usually the shortest route to an answer.

## When to reach for it

- The binary is protected and static reading is slow going.
- You need concrete values: a key, a decrypted config, an actual request body.
- You want to know which of several paths executes for a given input.
- The interesting logic sits behind a check that is cheaper to neuter than to understand.

Prefer static analysis when you need completeness (every path, not one), when execution is unsafe, or when the target refuses to run in your environment.

## Choosing a hook point

Hook where semantics are highest and obfuscation is lowest. Protection defends its own code, not the platform's:

- **Crypto primitives.** Cipher init/update/final gives you keys, IVs, and plaintext regardless of how the caller was obfuscated.
- **Allocation and copy.** `malloc`/`memcpy` with size logging reveals unpacked buffers and their contents.
- **Syscall or libc boundary.** File, socket, and process operations are ground truth for behavior.
- **Serialization.** Above encryption, below the wire — structured plaintext.
- **Runtime helpers.** For managed or JIT'd targets, the runtime's own resolve/load/compile functions see everything.

Hooking the target's own functions works too, but those are what a protector renames, inlines, and moves between versions.

## What to log

Arguments, return value, and enough context to correlate: thread, timestamp, and a short call-stack fingerprint. Hexdump buffers with a length cap. Log to a file, not the console — output volume surprises people.

Resist tracing everything. A log of every call produces megabytes nobody reads. Start narrow; widen only where the answer is not yet visible.

## Modifying behavior

Instrumentation is not read-only, and that is often the point. Replacing a return value defeats a check without understanding it: force an integrity verifier to report success, make a debugger check return false, stub a license validation. Note every modification so you can later distinguish the target's behavior from yours.

Calling functions directly is underused. Once you have a handle on an internal function and its signature, invoking it with your own arguments turns the target into a library — the fastest way to explore an encryption routine or a parser.

## Memory as evidence

Anything the process must hold in cleartext is in memory at some point. Dumping a region at the right moment yields unpacked code, decrypted configuration, and key material. The timing is the skill: after unpacking but before the payload clears its buffers, or at the first call into freshly written executable memory.

Scanning for known plaintext structure (PE headers, JSON braces, high-entropy blocks that just became low-entropy) locates the interesting region without guessing addresses.

## Anti-instrumentation

Targets check for hooks: scanning their own prologues for patches, resolving syscalls directly to bypass userland hooks, detecting injected modules, and validating timing. Counter by hooking deeper (kernel or emulator level), by patching the detection, or by using an emulator the target cannot introspect.

Timing checks are the most common nuisance — instrumentation is slow, and a `rdtsc` delta gives you away. Patch the timer read rather than trying to be fast.

## Emulation as an alternative

When a target will not run — wrong OS, missing hardware, aggressive environment checks — emulating just the interesting function is often viable. Set up the registers and memory a function expects, run it, read the result. This works well for isolated algorithms like a string decryptor or a checksum, and it sidesteps every anti-debug measure because nothing real is executing.

## Safety

Instrument in a disposable VM with controlled or absent network. A hooked malicious process is still a malicious process, and behavior modification can cause it to take paths its author never tested.
