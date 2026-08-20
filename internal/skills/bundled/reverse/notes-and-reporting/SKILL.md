---
name: notes-and-reporting
mode: reverse
summary: Structure findings so analysis survives the session and holds up to review.
---

# Notes and reporting

Reverse engineering produces understanding, which evaporates. The output that survives is what you wrote down, so treat notes as a deliverable rather than a byproduct.

## Write while working, not after

Every renamed function and recovered struct is a fact you will forget by tomorrow. Record it as you go:

- What the artifact is (hash, version, source).
- Named functions and what each does, one line each.
- Recovered structures and their field meanings.
- Constants, keys, and where they came from.
- What you tried that did not work — this is what saves the next session from repeating it.

A running log beats a polished document you never start.

## Separate verified from inferred

The single most important discipline. Three tiers:

- **Verified.** You ran it, decrypted it, or reproduced it. State the evidence.
- **Strongly inferred.** Constants match a known algorithm; a string names the behavior. Say what the inference rests on.
- **Guessed.** Plausible from context, unconfirmed. Label it as such or leave it out.

A confident wrong claim costs more than an acknowledged gap, because someone will build on it. When you cannot tell, say you cannot tell.

## Make claims falsifiable

"Encrypts data" is not useful. "AES-128-CBC with a key derived from the machine GUID via SHA-256, verified by decrypting `config.bin`" can be checked by someone else, and that is what makes it worth writing.

Include the address or file offset for anything you assert about code. A reader who disagrees needs to be able to look.

## Reproducibility

Prefer a script over manual steps. A decryption implemented as ten clicks in a debugger is not a finding anyone can reuse; the same thing as fifteen lines of code is. Where a tool's output mattered, record the exact invocation.

Save the artifacts: the unpacked binary, the extracted config, the decoded strings. Those are evidence, and regenerating them later is often harder than it was the first time.

## Structure of a final write-up

Lead with the conclusion. A reader wants the answer first and the derivation second:

1. **Summary.** What it is and what it does, in a few sentences.
2. **Key findings.** The things that matter, most important first.
3. **Technical detail.** Structures, algorithms, addresses, the reasoning.
4. **Open questions.** What remains unknown and why.
5. **Artifacts.** Scripts, extracted data, hashes.

For defensive work, add detection guidance: indicators, behavioral patterns, and signatures written against stable features rather than incidental ones.

## Annotate the project too

Notes in a document and annotations in the disassembler serve different purposes; do both. Comment intent in the project — why a function matters, what a constant means, which hypothesis a name encodes — so reopening it later starts from where you stopped rather than from the binary.

Export the project's symbol names and struct definitions alongside the notes. If the tool changes, that export is what carries the work forward.

## Scope your conclusions

Say what you analyzed, not what you assume about related things. A finding about one version of a binary is about that version. A behavior observed in a sandbox may differ in the wild. Note the boundary explicitly, because readers will otherwise generalize past it.
