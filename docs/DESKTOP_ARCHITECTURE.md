# Desktop architecture

[简体中文](DESKTOP_ARCHITECTURE.zh-CN.md)

Deepzora Desktop consists of a React UI, an Electron host, and a Go desktop service. These processes ship as one version unit. Electron owns native windows and browser views; Go owns business state and runtime execution.

The Go business layer is the importable `deepzora/desktop` module (package
`service`). Its historical directory also contains frontend and native shell
assets. `Service.Start`, `Invoke`, `SetEventSink`, `Contract` and `Shutdown` own
the shared application boundary; both the native host and `deepzora web` use it.
The executable entry is `desktop/cmd/desktop`; the CLI imports the service
directly. Both continue to use the same `internal/boot` / `control.Controller`
execution kernel as the terminal and ACP frontends.

`deepzora web` embeds the browser build of the same React UI in the CLI binary.
It serves an authenticated loopback origin and carries commands and events over
one WebSocket. The generated command contract, structured errors, secret
projection, session writer leases and approval owners are shared with Desktop.
Reloading a page never starts a second engine or takes over another process's
session. The new controlling page replaces the previous connection. Native
window/browser capabilities and automatic desktop integrations are disabled.

```text
React UI → typed preload IPC → Electron main → private stdio JSON-RPC → Go service
                                   │                              │
                                   ├─ browser views               ├─ controllers, sessions
                                   ├─ windows, tray, dialogs      ├─ leases, approvals, recovery
                                   └─ clipboard, notifications    └─ history, settings, files
```

| Layer | Responsibility |
| --- | --- |
| React | Render state projections and submit user intent through the typed bridge |
| Electron | Native lifecycle, browser views, menus, tray, dialogs, clipboard, notifications |
| Shared application service | Commands, controller ownership, sessions, approvals, history, settings and files; transport-specific native capabilities |
| Shared Go kernel | Providers, tools, persistence, leases, recovery and billing |
| Web adapter | Loopback authentication, same-origin validation, browser commands and events; no session business logic |

The reflected `DesktopContract` generates the command table, DTO declarations and handshake digest. Events carry sequence, generation, name and arguments. Go delegates native operations through `NativeHost`; browser tools use the host capability registry and per-session grants. See [host protocol](DESKTOP_HOST_PROTOCOL.md), [browser contract](DESKTOP_BROWSER.md), and [generated inventory](desktop-contract/INVENTORY.md).

Unsent session input stays separate from committed transcripts. Runtime and attachment mutations are scoped to the originating session and revision. Late work from replaced tabs cannot acquire the current session's authority. Transcript rendering retains its single viewport writer and bounded history window; see [scroll contract](TRANSCRIPT_SCROLL_CONTRACT.md).

## Verification

Build and test both Go modules, regenerate the host contract, run frontend and Electron tests, and verify the generated inventory. Browser suites exercise new-session creation, input recovery, approvals and transcript navigation. Release candidates additionally require native signing and acceptance checks on each supported platform; local browser tests or cross-compilation do not establish native platform qualification.

Use [Contributing](../CONTRIBUTING.md) for development commands and [release candidates](RELEASE_CANDIDATES.md) for publication gates. The application service supports the current Electron host and local Web transport on the current wire/storage contracts.
