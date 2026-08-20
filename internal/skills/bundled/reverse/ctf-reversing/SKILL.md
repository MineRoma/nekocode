---
name: ctf-reversing
mode: reverse
summary: Time-efficient tactics for reverse-engineering CTF challenges.
---

# CTF reversing

CTF binaries are small, intentional, and solvable. The optimization target is time, not thoroughness — a challenge has one intended path and usually one shortcut.

## Two-minute triage

Before reading code, exhaust the free wins:

1. `strings` — the flag itself is sometimes present, and library names reveal the language.
2. `file` and section listing — format, architecture, stripped or not, statically or dynamically linked.
3. Run it. See what it asks for and what it prints on success and failure. The failure string is a grep target that leads directly to the comparison.
4. Check for anti-debugging early, so you know whether dynamic analysis is available.

## Recognize the archetype

Most challenges are one of a handful of shapes:

- **Direct comparison:** input compared against a stored string. Read the string.
- **Transform then compare:** input transformed (XOR, arithmetic, reordering) and compared to a constant. Invert the transform.
- **Character-by-character check:** each byte validated independently — brute-force one byte at a time, which is linear rather than exponential.
- **Checksum or hash:** input must produce a target value. Either the function is invertible, or the search space is small, or the intended solution is a solver.
- **VM interpreter:** the binary implements a small VM and the flag check is bytecode. Reverse the opcode handlers first, then write a disassembler for the bytecode.
- **Multi-stage:** an unpacking or decryption stage produces the real challenge. Dump after the stage runs.

Identifying the archetype tells you which tool to reach for. Most wasted time in CTF reversing is spent reading code that a solver would have handled.

## Symbolic execution

For transform-then-compare and checksum challenges, a symbolic engine often solves the whole thing: mark the input symbolic, constrain the success path, ask the solver for a satisfying input. This works without understanding the algorithm at all, which is exactly the point when time is scored.

It struggles with heavy loops, hashing, and large state, so recognize when to fall back to manual analysis.

## Instrumentation shortcuts

- Breakpoint the comparison and read the expected value from memory.
- Patch the check to always succeed, then let the program print the flag itself — sometimes the flag is derived, not compared.
- Trace executed instructions with varied inputs and diff the traces; the point where they diverge is the check.
- Count executed instructions as a side channel for byte-at-a-time brute force.

## Common encodings

XOR with a single byte or repeating key, base64 variants including custom alphabets, ROT and Caesar shifts, byte-order swaps, and simple substitution tables. Recognizing these on sight saves minutes each.

Flags follow a known format, which gives you known plaintext at a known position — often enough to recover a key immediately.

## Discipline

Note what you have ruled out, not just what you have found. Keep the working script in one file so a partial solution is re-runnable. When stuck for more than fifteen minutes on one approach, switch archetypes — the intended path is usually simpler than the one you are on.
