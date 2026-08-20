---
name: api-reverse-engineering
mode: reverse
summary: Recover undocumented HTTP and RPC APIs from clients and traffic.
---

# API reverse engineering

Recovering a private API is observation plus client reading. The client is a complete specification of the API's usable surface; traffic tells you what it actually sends.

## Capture first

Put a proxy between client and server. For browsers that is devtools; for mobile and desktop clients it is an intercepting proxy with a trusted certificate.

Certificate pinning blocks interception. Defeat it at the client — patch the pin check or hook the trust manager — rather than attacking TLS. See `dynamic-instrumentation`.

For non-HTTP transports (gRPC, WebSocket, custom binary framing), see `protocol-analysis`.

## Read the client

The client contains every endpoint it can reach. Look for base URL constants, path fragments at call sites, and a central request helper — that helper is where authentication, headers, and signing live, so reading it in full pays for itself.

Mobile and web clients often ship a generated API layer with the schema effectively intact: retrofit interfaces, OpenAPI-derived models, or GraphQL documents. Recovering those gives you the full endpoint catalog, request and response types, and authentication patterns in one pass.

## Authentication and signing

The highest-value target in most private APIs. Identify: how tokens are obtained, what they are (JWT, opaque, HMAC-signed), how they attach (header, cookie, query), whether requests carry a signature, and what key material the signature needs. If the signing secret is derived from a constant in the client, you can replicate it. If it comes from the server, you need a real token.

## What to document

For each endpoint: method, path, required headers, request body shape, response shape, error codes, and any rate-limiting or pagination you observe. A working request example is worth paragraphs of description.

For the API overall: the auth flow, the base URL pattern (versioning), the content type, and any global headers or signing. An OpenAPI or similar machine-readable spec is the ideal end product if the API is large enough to warrant it.

## Authorization

Reverse a client you own, a client in a test engagement, or a client whose API you intend to consume for interoperability. Using a reversed API to scrape, abuse, or bypass authorization you are not entitled to is a separate decision with separate consequences — the technique does not distinguish the two; your judgment must.