---
name: electron-apps
mode: reverse
summary: Extract and analyze Electron applications and their bundled JavaScript.
---

# Electron applications

An Electron app is Chromium plus Node.js plus application JavaScript. The application code is not compiled — it is packaged, and packaging is trivially reversible.

## Extraction

Application code lives in `app.asar`, an uncompressed archive with a JSON header describing file offsets. Extract it and you have the source tree, often with the original directory structure and sometimes source maps. Look in `resources/` in the install directory.

`app.asar.unpacked` holds files that had to stay on disk — typically native `.node` modules, which are ordinary shared libraries requiring native analysis.

`package.json` names the entry point, the Electron version, and the dependencies.

## Where to look

- **Main process** (the entry point): window creation, IPC handlers, `nodeIntegration` and `contextIsolation` settings, auto-update configuration, and direct filesystem or network access.
- **Preload scripts:** the bridge between renderer and Node. What a preload exposes via `contextBridge` defines exactly what the web content can reach.
- **Renderer:** the UI, usually a bundled front-end app. Apply the `js-deobfuscation` skill — it is typically webpack output.
- **IPC channels:** the internal API between processes. Enumerating channel names and their handlers maps the app's privileged surface.

## Bytecode-compiled code

Some apps ship V8 bytecode (`.jsc`) instead of source to deter reading. It is not encryption: the bytecode is version-locked to the exact V8 build, and the source strings, function names, and constants are largely still present. Extraction tools exist; failing that, the constant pool alone often answers the question.

## Runtime inspection

The DevTools protocol is built in. Enabling it — via a command-line flag, an environment variable, or a patched main script — gives you the full debugger against the running app: breakpoints, network inspection, and a console with access to the renderer context.

For the main process, Node's inspector applies. Since you can edit the extracted `app.asar` and repack it, adding your own logging or a debugger statement is straightforward.

## Security-relevant findings

Because the boundary between web content and Node is configurable, misconfigurations are the common finding: `nodeIntegration: true` with remote content, disabled `contextIsolation`, overly broad `contextBridge` exposure, missing update signature verification, and secrets committed into the bundle. These are worth reporting explicitly when auditing.

## Practical order

1. Extract `app.asar`; note `app.asar.unpacked` contents.
2. Read `package.json`, find the main entry.
3. Read the main process for window and IPC configuration.
4. Read preload scripts to see the exposed surface.
5. Deobfuscate the renderer bundle if needed.
6. Attach DevTools for dynamic questions.
