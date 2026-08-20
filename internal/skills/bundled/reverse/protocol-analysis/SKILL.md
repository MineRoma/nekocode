---
name: protocol-analysis
mode: reverse
summary: Recover undocumented wire formats from captures and client code.
---

# Protocol reverse engineering

Two sources of truth: the bytes on the wire, and the code that produces them. Use both — captures show what actually happens, code shows what is possible.

## Start from the capture

Collect many samples of the same operation before theorizing. One capture tells you almost nothing; twenty captures of the same action with varied inputs tell you which bytes are constant, which are input-dependent, and which change every time.

Then diff:

- **Constant across all captures:** magic numbers, version fields, framing.
- **Varies with your input:** the payload you control. Change one thing and see which bytes move.
- **Varies every time regardless:** timestamps, sequence numbers, nonces, session IDs, random padding.
- **Varies with length:** length prefixes and checksums. Verify by changing payload size by one byte.

## Framing first

Nothing else can be parsed until you know where messages begin and end. The common schemes: fixed-size headers, length-prefixed payloads (note the endianness and whether the length covers the header), delimiter-terminated, and self-describing type-length-value chains. TLV is the most common in practice and the easiest to confirm — the same three-part pattern repeats visibly.

## Field typing

Once framed, classify each field. Integers reveal their endianness across varied values. Timestamps look like plausible epoch values. Length fields correlate with the size of what follows. Enums take few distinct values across many captures. Strings may be length-prefixed or NUL-terminated. Bit flags show as single bits toggling with UI state.

Confirm hypotheses by prediction: construct a message yourself and see whether the server accepts it. A format you can generate is a format you understand.

## Serialization frameworks

Check for a known framework before hand-rolling a parser. Protobuf, MessagePack, CBOR, BSON, Thrift, and Avro all have recognizable signatures, and protobuf in particular can be partially decoded without the schema — field numbers and wire types are self-describing, so you recover the structure and only have to guess names.

Recovering the `.proto` from the client binary is usually possible: descriptors are frequently embedded verbatim.

## When it is encrypted

Do not attack the crypto. Attack the endpoints, where plaintext necessarily exists:

- Hook the encryption function in the client and log its input.
- Use a proxy with a trusted certificate, defeating pinning by patching the check rather than the TLS.
- Read the key material from client memory — it has to be there.
- Instrument the serialization layer, which sits above encryption and sees structured plaintext.

## From the code

The send path in the client serializes structured data into bytes. Follow it backwards: the struct or object passed to the send function defines the message layout. Field names and types in source are the schema you are recovering.

The receive path deserializes — it tells you what the other side sends, and how errors are handled.

## Documenting the result

A protocol spec is: the framing rule, the message types by ID, the field layout per message type, endianness, and examples. Write it down in something parseable (a table, a proto-like schema, or a working encoder/decoder) rather than only in prose. A decoder that produces human-readable output from a capture is both the documentation and the test.