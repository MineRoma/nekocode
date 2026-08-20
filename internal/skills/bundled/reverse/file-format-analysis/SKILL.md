---
name: file-format-analysis
mode: reverse
summary: Reverse undocumented file formats, archives, and save data.
---

# File format analysis

An undocumented file format is a protocol that does not move. The same differential method applies, and you have an advantage: you can usually generate as many samples as you want.

## Generate, then diff

If the producing application is available, make it emit files that differ in exactly one way. Save a document with one character changed. Export with one setting toggled. Create an archive with one file added.

The diff between two such samples localizes that field precisely. This is far faster than staring at a single hexdump, and it is the whole technique in one sentence.

## Read the header

Almost every format starts with a magic number, a version, and offsets or sizes. Identify:

- **Magic.** Four bytes is typical. It tells you whether the format is known.
- **Version.** Explains why old and new files differ.
- **Endianness.** Compare a value you know (a count you control) against both interpretations.
- **Offset table or directory.** Many formats are a header pointing at chunks. Finding the directory turns one opaque blob into a list of smaller ones.

## Chunked formats

The most common structure in practice: a sequence of `[type][length][data]` records. Once you recognize it, parsing is mechanical and unknown chunk types can be skipped safely — which is exactly why designers use it.

RIFF, PNG, ELF sections, and countless game formats all follow this shape. Look for a repeating pattern where a 4-byte value predicts the distance to the next similar pattern.

## Compression and encoding

High entropy in a data region usually means compression rather than encryption. Check for known signatures first: zlib headers (`78 01`, `78 9C`, `78 DA`), gzip magic, LZ4, Zstd, and Deflate raw streams. Trying a decompressor is cheap and definitive.

Custom LZ variants are common in games. They are recognizable in the decompressor: a loop reading control bytes, then either copying literals or back-referencing earlier output. Reversing the decompressor is the reliable path, and it doubles as a compressor once you understand it.

## When the format is versioned

Old and new files of the same type give you a free view of the format's evolution — fields that were added, sizes that grew. If the application supports old versions, its loader contains explicit version branches, which is effectively documented structure.

## Reading the parser instead

The producing or consuming application contains a complete, authoritative specification: its parser. Finding the load function and reading it top to bottom gives you field names, types, and validation rules — everything a hexdump makes you guess. When the application is available, this beats pure differential analysis.

Look for the file-open call and follow the buffer.

## Validating your understanding

Write a parser and run it over many real files. Every file that parses cleanly to the end confirms the structure; the first file that fails shows you what you missed. Then write a writer and check the application accepts your output — a format you can generate is a format you understand.

Round-tripping (parse then re-emit, compare bytes) catches fields you silently ignored.

## Documenting

Produce a structure definition, not prose: a table of offsets, sizes, types, and meanings, plus the chunk-type list. A working parser is the best documentation, and a hex template for your editor of choice makes future samples readable at a glance.
