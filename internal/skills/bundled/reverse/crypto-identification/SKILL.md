---
name: crypto-identification
mode: reverse
summary: Recognize cryptographic algorithms and custom obfuscation in binaries.
---

# Crypto identification

Cryptographic code is unusually recognizable, because implementations converge on the same constants and the same loop shapes. Identifying the algorithm turns an opaque function into a known quantity with known weaknesses.

## Constants are fingerprints

Most standard algorithms carry unmistakable magic values:

- **MD5:** initial state `67452301 EFCDAB89 98BADCFE 10325476`, plus a 64-entry sine table.
- **SHA-1:** `5A827999 6ED9EBA1 8F1BBCDC CA62C1D6` round constants.
- **SHA-256:** initial state starting `6A09E667 BB67AE85`, plus 64 round constants beginning `428A2F98`.
- **AES:** a 256-byte S-box starting `63 7C 77 7B F2 6B 6F C5`, and the round-constant sequence `01 02 04 08 10 20 40 80 1B 36`.
- **DES:** permutation tables of characteristic sizes.
- **CRC32:** a 256-entry table starting `00000000 77073096 EE0E612C`.
- **Blowfish:** a large table of digits of pi.
- **ChaCha20/Salsa20:** the ASCII constant `expand 32-byte k`.
- **RSA and ECC:** big-integer routines plus standard curve parameters (P-256 and Curve25519 constants are published and searchable).

Searching for these tables is the single highest-yield step. Signature databases automate it.

## Structural recognition without constants

When constants are computed at runtime rather than stored, recognize the shape:

- **Block ciphers:** a fixed number of rounds over a fixed-size block, with a key schedule computed once and reused.
- **Stream ciphers:** a keystream generator XORed against data of arbitrary length.
- **Hashes:** a compression function over fixed-size chunks with padding that encodes the original length.
- **Public key:** modular exponentiation or curve point arithmetic on multi-word integers.
- **HMAC:** a hash called twice with two derived keys and constant padding bytes `0x36`/`0x5C`.

Note the block size, the number of rounds, and the key size — those three usually identify the algorithm even without constants.

## Custom and modified crypto

Two distinct cases:

**Standard algorithm, modified constants.** A tweaked S-box or altered initial state. The structure is unchanged, so it is still recoverable and still as strong as the original — just non-interoperable. Recover the modified tables and reimplement.

**Home-grown obfuscation.** XOR with a repeating key, byte substitution, rotation, addition chains, or a combination. Common in malware configuration and game save files. Almost always weak: recover the algorithm and it is fully reversible. Look for a small loop with arithmetic on a byte and an index-derived value.

Do not mistake obfuscation for encryption in your report. "XOR with a 4-byte key" and "AES-256-CBC" have very different implications, and the code often looks superficially similar.

## Finding the key

The key is the actual objective in most cases, and it must exist somewhere:

- Stored in the binary as a literal, sometimes obfuscated by a layer of the same kind.
- Derived from a password by a KDF — identify the KDF and its parameters.
- Derived from machine or environment values (hardware IDs, timestamps, hostname), which is how per-victim ransomware and machine-locked licensing work.
- Fetched from a network endpoint, in which case the endpoint is the finding.
- Present only in memory at runtime — dump it there.

## Mode of operation

Identify the mode, not just the primitive: ECB (identical blocks produce identical ciphertext, visible in a hexdump), CBC (an IV and chaining), CTR (a counter turning a block cipher into a stream cipher), GCM (authentication tag alongside ciphertext).

The mode determines whether ciphertext can be manipulated and whether identical plaintexts are detectable, both of which matter for what you can do with your finding.

## Reporting

State the algorithm, key size, mode, where the key comes from, and how you verified it. Verification means decrypting real data — a decrypted sample is proof; a constant match is a strong hypothesis. Distinguish the two.
