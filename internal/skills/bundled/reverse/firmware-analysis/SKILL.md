---
name: firmware-analysis
mode: reverse
summary: Extract and analyze embedded firmware images and bare-metal code.
---

# Firmware analysis

A firmware image is a filesystem, a kernel, a bootloader, or all three concatenated. The first task is separating them; the second is deciding which one holds the answer.

## Extraction

Run a signature-carving pass over the whole image first. It finds embedded filesystems (SquashFS, JFFS2, CramFS, UBI), compressed blobs, and certificates by magic bytes, and it usually decomposes the image with no manual work.

When carving yields a filesystem, you likely have a Linux-based device and the interesting code is userland binaries — analyze them as ordinary ELF targets.

When carving yields nothing but one large blob, you have a bare-metal or RTOS image: no symbols, no sections, and you must determine the load address and architecture yourself.

## Bare-metal orientation

Establish three things before disassembling:

1. **Architecture.** ARM (and Thumb), MIPS, RISC-V, or a microcontroller core. Instruction-pattern heuristics identify it; the vendor datasheet confirms it.
2. **Load address.** A wrong base means every absolute reference points at nothing. Recover it by finding a table of pointers sharing a high prefix — that prefix is your base. The vector table is the usual source.
3. **Vector table.** On ARM Cortex-M the image starts with the initial stack pointer followed by the reset vector and exception handlers, which hands you the entry point immediately.

## What to look for

- **Hardcoded credentials.** Still common. Grep for username-shaped strings and default-config blocks.
- **Keys and certificates.** Private keys shipped in firmware are a recurring finding, and PEM headers are grep-able.
- **Update mechanism.** Whether the device validates an update signature determines whether you can supply your own image — the highest-impact thing to check.
- **Debug interfaces.** Leftover UART consoles, telnet daemons, undocumented commands. Strings often name them outright.
- **Memory-mapped I/O.** In bare-metal code, accesses to fixed high addresses are peripherals. The datasheet's register map turns those into meaningful operations, which is what makes the code readable at all.

## Hardware as a shortcut

Software-only analysis is slower than reading a console. If you have the physical device, a UART header usually exists on the board and frequently offers a bootloader prompt or a root shell. JTAG or SWD, when not fused off, gives full memory access and single-stepping, which beats any static approach.

Dumping flash directly gets you the image when the vendor does not publish one.

## Encrypted images

Vendors increasingly encrypt firmware. In rough order of effort: find the decryption key in a previous unencrypted release, extract it from the bootloader (which must decrypt and is often itself unencrypted), read it from the chip over a debug interface, or intercept the plaintext in RAM after the device decrypts it.

An update that the device accepts must be decryptable by the device, so the key is reachable.

## Filesystem findings

For Linux-based images, the startup scripts define behavior more than any single binary: what services launch, what ports open, what runs as root. Read `/etc/init.d`, the inittab, and any vendor startup script before opening a disassembler.

Web interfaces on embedded devices are usually CGI binaries; those are where authentication bypasses and command injection live.

## Practical order

1. Carve the image; identify what came out.
2. Filesystem present → analyze startup scripts, then userland binaries.
3. Single blob → determine architecture, load address, and vector table.
4. Grep for credentials, keys, and debug strings.
5. Read the update mechanism and its signature verification.
6. Use hardware access (UART, JTAG) if available — it is usually faster than everything above.

## Authorization

Analyze devices you own, devices you are engaged to test, and published firmware for research. Vendor terms sometimes prohibit reverse engineering; jurisdiction and purpose matter, and security research exemptions are narrower than people assume. For findings in someone else's product, use coordinated disclosure.