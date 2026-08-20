---
name: android-apk
mode: reverse
summary: Analyze APKs from manifest to native libraries, statically and with instrumentation.
---

# Android APKs

An APK is a zip with predictable contents. That structure makes the first pass mechanical, and the manifest tells you where to look.

## Static pass

1. **Manifest.** Binary XML — decode it. It names every component (activities, services, receivers, providers), the permissions, the `minSdk`/`targetSdk`, whether `debuggable` and `allowBackup` are set, the network security config, and any exported components. Exported components are the attack surface.
2. **DEX.** `classes.dex` (plus `classes2.dex`…) is Dalvik bytecode. A DEX-to-Java decompiler gives readable output; when it fails, Smali is a faithful and workable intermediate. Multi-DEX is normal for large apps — check every file.
3. **Resources and assets.** Hardcoded endpoints, API keys, pinned certificates, and embedded databases live here. `res/raw` and `assets/` are the usual places.
4. **Native libraries.** `lib/<abi>/*.so` are ELF shared objects. The JNI boundary is where Java meets native: either `Java_com_example_Class_method` exports, or dynamic registration via `RegisterNatives` in `JNI_OnLoad`. Dynamic registration hides the mapping, so read `JNI_OnLoad` to recover it.
5. **Signature.** `META-INF` and the signing block identify the signer and the signature scheme version.

## What obfuscation looks like

R8/ProGuard renames classes and methods to single letters and prunes unused code. This is near-universal in release builds and is not by itself a sign of malice. Work from the parts that cannot be renamed: framework API calls, manifest-declared component names, resource identifiers, and string literals.

Commercial packers go further — the real DEX is encrypted and loaded at runtime by a native stub. Extract it from memory after the loader runs rather than fighting the stub.

## Dynamic instrumentation

For most real questions, instrumentation beats static reading. A hooking framework intercepts any Java or native method in a running app: log arguments and return values, replace return values, call methods directly.

High-value hooks:

- **Crypto:** cipher init and update calls, to capture keys, IVs, and plaintext.
- **Network:** the HTTP client layer above TLS, to read requests and responses in the clear.
- **Certificate pinning:** the trust manager or the pinning check, to enable proxy interception.
- **Root and tamper checks:** the detection methods, to return expected values.
- **JNI:** `RegisterNatives`, to recover the dynamic method mapping automatically.

## Reversing native libraries

Treat the `.so` as a normal ELF target, with JNI specifics: the first parameter is `JNIEnv*`, the second is the `jobject` or `jclass`, and everything the native code does to Java objects goes through `JNIEnv` function-table offsets. Recovering that table turns opaque indirect calls into named API calls, which is the single most useful step.

Obfuscated native libraries are common in games and protection SDKs — combine with the `anti-reversing` skill.

## Practical order

1. Unzip, decode the manifest, enumerate components and permissions.
2. Decompile DEX, locate the entry activity and any exported components.
3. Grep resources and assets for endpoints and secrets.
4. Identify native libraries and their JNI entry points.
5. Instrument the running app for anything static analysis leaves unclear.

## Authorization

Analyze apps you own, apps you are engaged to test, malware samples, and CTF targets. Extracting paid content or bypassing licensing in someone else's app is a different activity from security analysis, regardless of the technique overlap.
