---
name: symbolic-execution
mode: reverse
summary: Solve for inputs with constraint solvers instead of reading algorithms.
---

# Symbolic execution

Instead of understanding what a function computes, describe it as constraints and ask a solver for an input that satisfies your goal. This answers "what input reaches this state" without reading the algorithm at all.

## When it wins

- A validation routine you need to pass, where the transform is invertible in principle but tedious by hand.
- A checksum or magic-value check with a small solution space.
- Path discovery: which inputs reach a particular branch.
- CTF challenges of the transform-then-compare shape, where it often solves the whole problem.

It works because the solver does the algebra. Your job is scoping.

## When it fails

Be honest about the limits, or you will burn hours:

- **Path explosion.** Loops with input-dependent bounds multiply states until memory is gone.
- **Cryptographic hashes.** Deliberately unsolvable — a solver will not invert SHA-256, and asking it to is a mistake.
- **Large symbolic memory.** Symbolic indices into large buffers blow up quickly.
- **Environment interaction.** Syscalls, file I/O, and network calls need modeling, and imperfect models produce wrong answers.

If the target has any of these, either constrain harder or switch approaches.

## Scoping is the skill

Do not symbolize the whole program. Isolate the check:

1. Find the function that decides success — usually near a "wrong" or "invalid" message.
2. Start execution at that function rather than at `main`, with concrete state for everything you know.
3. Symbolize only the input bytes, and constrain them tightly: length, printable range, known prefix. Every constraint you add shrinks the search enormously.
4. State the goal as reaching the success branch, or as a return value equality.
5. Ask for a model.

A tightly scoped query solves in seconds where a whole-program query never terminates.

## Concolic execution

Mixing concrete and symbolic values sidesteps most explosion problems: run with a real input, record the path taken, then flip one branch condition and solve for an input that takes the other. This explores incrementally and keeps state small.

It is the practical default for anything nontrivial, and it composes well with instrumentation — run under a tracer, then solve against the recorded trace.

## Verifying the answer

A solver's model is a claim, not a result. Feed it to the real binary and check that it actually succeeds. Models are frequently correct-per-your-constraints but wrong for the program, because the constraints omitted something — an unmodeled syscall, an ignored global, a wrong assumption about length.

When verification fails, the constraints were incomplete. That is information: find what you missed.

## Combining with static analysis

Symbolic execution is best as a targeted tool inside manual analysis, not a replacement for it. Read enough to find the check and understand its inputs, solve the check mechanically, and go back to reading. Analysts who treat it as a first resort on whole binaries mostly generate timeouts.
