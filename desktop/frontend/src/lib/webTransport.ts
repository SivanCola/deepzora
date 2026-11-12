import type { AppBindings } from "./bridge";
import type { DesktopHost } from "./desktopHost";
import { DESKTOP_COMMANDS, DESKTOP_CONTRACT_DIGEST, DESKTOP_PROTOCOL_VERSION } from "../generated/desktopContract.generated";

type Reply = {
  ready?: boolean; protocolVersion?: number; digest?: string;
  event?: string; args?: unknown[]; id?: number; value?: unknown;
  error?: { message: string; code?: number; data?: unknown };
};

// No retries of accepted mutations: a lost connection may have committed them.
// Reload reconstructs the view from the same authoritative backend/session.
export class WebConnection {
  readonly ready: Promise<void>;
  private resolveReady!: () => void;
  private rejectReady!: (error: Error) => void;
  private nextID = 0;
  private pending = new Map<number, { resolve(value: unknown): void; reject(error: Error): void }>();
  private listeners = new Map<string, Set<(...args: unknown[]) => void>>();
  private failure: Error | null = null;

  constructor(private socket: WebSocket, private onFailure: (error: Error) => void) {
    this.ready = new Promise((resolve, reject) => { this.resolveReady = resolve; this.rejectReady = reject; });
    // The first consumer can be mounted after an early connection error.
    void this.ready.catch(() => {});
    const timeout = setTimeout(() => this.fail(new Error("Web connection timed out. Reload this page.")), 15000);
    void this.ready.then(() => clearTimeout(timeout), () => clearTimeout(timeout));
    socket.onmessage = (event) => {
      let reply: Reply;
      try { reply = JSON.parse(String(event.data)) as Reply; }
      catch { this.fail(new Error("Invalid Web service response. Reload this page.")); return; }
      if (reply.ready) {
        if (reply.protocolVersion !== DESKTOP_PROTOCOL_VERSION || reply.digest !== DESKTOP_CONTRACT_DIGEST) {
          this.fail(new Error("Web UI and service versions differ. Restart deepzora web and reload.")); return;
        }
        this.resolveReady();
      } else if (reply.event) {
        for (const cb of this.listeners.get(reply.event) ?? []) cb(...(reply.args ?? []));
      } else if (reply.id) {
        const pending = this.pending.get(reply.id);
        if (!pending) return;
        this.pending.delete(reply.id);
        if (reply.error) pending.reject(Object.assign(new Error(reply.error.message), { code: reply.error.code, data: reply.error.data }));
        else pending.resolve(reply.value);
      }
    };
    socket.onclose = () => this.fail(new Error("Web connection closed. Tasks remain on the server. Reload this page to reconnect; only one page can control this workspace."));
    socket.onerror = () => this.fail(new Error("Cannot connect to Deepzora Web. Keep deepzora web running and open the access URL printed in the terminal."));
  }

  private fail(error: Error) {
    if (this.failure) return;
    this.failure = error;
    this.rejectReady(error);
    for (const pending of this.pending.values()) pending.reject(error);
    this.pending.clear();
    this.socket.close();
    this.onFailure(error);
  }

  async invoke(method: string, args: unknown[]): Promise<unknown> {
    await this.ready;
    if (this.failure) throw this.failure;
    const id = ++this.nextID;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      try { this.socket.send(JSON.stringify({ id, method, args })); }
      catch (error) { this.pending.delete(id); reject(error); }
    });
  }

  on(name: string, cb: (...args: unknown[]) => void): () => void {
    const listeners = this.listeners.get(name) ?? new Set();
    this.listeners.set(name, listeners);
    listeners.add(cb);
    return () => { listeners.delete(cb); };
  }
}

function showConnectionFailure(error: Error) {
  if (document.getElementById("web-connection-error")) return;
  const overlay = document.createElement("div");
  overlay.id = "web-connection-error";
  overlay.setAttribute("role", "alertdialog");
  overlay.setAttribute("aria-label", "Web connection unavailable / Web 连接不可用");
  overlay.style.cssText = "position:fixed;inset:0;z-index:2147483647;background:#111214ee;color:#f4f4f3;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:20px;padding:32px;font:16px system-ui;text-align:center";
  const text = document.createElement("p");
  text.textContent = "Web 连接不可用，请确认终端服务仍在运行，再重新加载。\n" + error.message;
  const reload = document.createElement("button");
  reload.textContent = "重新加载 / Reload";
  reload.style.cssText = "padding:10px 20px;border:0;border-radius:8px;background:#f97316;color:#111;cursor:pointer";
  reload.onclick = () => window.location.reload();
  overlay.append(text, reload);
  document.body.append(overlay);
}

let webHost: DesktopHost | undefined;
export function localWebHost(fallback: DesktopHost): DesktopHost | undefined {
  if (typeof document === "undefined" || !document.querySelector('meta[name="deepzora-web"]')) return undefined;
  if (webHost) return webHost;
  document.documentElement.dataset.host = "web";
  // Capture the fragment before any other code can copy it to storage or links.
  const token = new URLSearchParams(window.location.hash.slice(1)).get("token");
  if (token) history.replaceState(null, "", window.location.pathname + window.location.search);
  const connected = (async () => {
    if (token) {
      const auth = await fetch("/web/auth", { method: "POST", headers: { Authorization: `Bearer ${token}` }, credentials: "same-origin" });
      if (!auth.ok) throw new Error("Access URL rejected. Open the current URL from the terminal.");
    }
    const socket = new WebSocket(`ws://${window.location.host}/web/socket`);
    const connection = new WebConnection(socket, showConnectionFailure);
    await connection.ready;
    return connection;
  })();
  void connected.catch(showConnectionFailure);
  const invoke = async (method: string, args: unknown[]) => (await connected).invoke(method, args);
  webHost = {
    ...fallback,
    kind: "web",
    app: new Proxy({} as AppBindings, {
      get: (_target, prop) => {
        if (typeof prop !== "string" || !(DESKTOP_COMMANDS as readonly string[]).includes(prop)) return undefined;
        if (prop === "PickWorkspace") return async () => {
          const path = window.prompt("项目目录的绝对路径 / Absolute project directory");
          return path?.trim() ? invoke("SwitchWorkspace", [path.trim()]) : "";
        };
        return (...args: unknown[]) => invoke(prop, args);
      },
    }),
    events: { on: (name, cb) => {
      let off: (() => void) | undefined;
      let disposed = false;
      void connected.then((connection) => { if (!disposed) off = connection.on(name, cb); }).catch(() => {});
      return () => { disposed = true; off?.(); };
    } },
    native: {
      ...fallback.native,
      clipboardWriteText: async (text) => { await navigator.clipboard.writeText(text); return true; },
      clipboardReadText: () => navigator.clipboard.readText(),
    },
  };
  return webHost;
}
