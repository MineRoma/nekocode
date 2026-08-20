---
name: anti-reversing
mode: reverse
summary: Recognize and defeat packers, obfuscators, and anti-analysis checks.
---

# Anti-reversing and software protection

Protection exists on a spectrum. Identify where a sample sits before choosing an approach, because the effort difference between tiers is enormous.

## Tiers

**Tier 1 — compression packers** (UPX, ASPack). Reversible. Often a single command, or dump-after-unpack at the original entry point.

**Tier 2 — obfuscators** (control-flow flattening, opaque predicates, string encryption, junk instructions). The original code is still present, just buried. Defeat with deobfuscation passes and symbolic simplification.

**Tier 3 — virtualizing protectors** (VMProtect, Themida, Code Virtualizer). Original instructions are replaced by bytecode for a custom VM embedded in the binary. You must recover the VM's handler table and lift its bytecode back to native semantics. This is a project, not a task.

## Identifying anti-analysis checks

Look for these before you run anything:

- **Debugger detection:** `IsDebuggerPresent`, `CheckRemoteDebuggerPresent`, PEB `BeingDebugged` flag read directly at `fs:[0x30]+2`, `NtQueryInformationProcess` with `ProcessDebugPort`, timing checks around `rdtsc`.
- **VM detection:** CPUID hypervisor bit, MAC address prefixes, registry keys naming VMware/VirtualBox, disk size and RAM thresholds, driver file existence.
- **Integrity checks:** self-hashing of `.text`, checksum comparison, breakpoint detection by scanning for `0xCC` bytes.
- **Anti-dumping:** erased PE headers, guard pages, `NtSetInformationThread` with `ThreadHideFromDebugger`.

Patch checks out rather than satisfying them — one `jz`→`jmp` is faster than faking a plausible environment. Keep a list of every patch so you can tell later which behavior is the sample's and which is yours.

## Control-flow flattening

The pattern is a dispatcher loop with a state variable selecting among many basic blocks. Recover it by identifying the state variable, enumerating each block's effect on it, and rebuilding the real edges. The result is the original CFG. Tooling helps, but the state variable must be found by reading.

## Opaque predicates

Conditions that always evaluate one way, inserted to create dead branches. Recognize the algebraic identities (`(x*x - x) % 2 == 0`, `x | 1 != 0`). A symbolic execution engine or an SMT solver proves the branch constant, after which the dead side can be removed.

## String encryption

Strings decrypt at use, so the decryption routine is a chokepoint. Find it, then either script the decryption offline across the whole table or hook it dynamically and log every plaintext. Annotate the results back into the disassembly — otherwise you lose them.

## Virtualized code

Recovering a VM is a structured process: find the VM entry, identify the dispatcher and the handler table, reverse each handler to a semantic operation, decode the bytecode stream, then translate it back. Devirtualization tools exist for known protectors and specific versions; they are version-fragile, so verify their output against the running binary rather than trusting it.

Consider whether you need full devirtualization. If the goal is a single algorithm or key, targeted dynamic instrumentation of the VM's memory accesses often answers the question in a fraction of the time.

## Dynamic analysis as the escape hatch

Static analysis of a heavily protected binary can cost more than it returns. Protection defends the code, not the behavior: the sample must still decrypt, still allocate, still send. Instrumenting at that boundary — API hooks, memory dumps at the right moment, network capture — gets the answer while sidestepping the protection entirely.

## Authorization

Analyzing protection is legitimate work in malware analysis, CTFs, interoperability, security research, and testing your own products. Bypassing licensing on software you have no rights to is not, and the fact that the technique is identical does not make the two the same. Know which one you are doing.
