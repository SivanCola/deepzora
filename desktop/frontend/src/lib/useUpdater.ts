import { createContext, createElement, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { app, onUpdaterProgress } from "./bridge";
import type { UpdateInfo } from "./types";

// useUpdater drives the auto-update state machine shared by the top banner and the
// Settings panel. It uses a single "update and restart" action that downloads,
// verifies, installs, and relaunches. There is no durable cross-restart pending
// state: failures leave the current version running and the user simply retries.

export type UpdateStatus =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "upToDate"; current: string }
  | { kind: "available"; info: UpdateInfo }
  | { kind: "downloading"; received: number; total: number; info: UpdateInfo }
  | { kind: "verifying"; info: UpdateInfo }
  | { kind: "authorizing"; info?: UpdateInfo }
  | { kind: "installing"; info?: UpdateInfo }
  | { kind: "relaunching"; info?: UpdateInfo }
  | { kind: "done" }
  | { kind: "error"; message: string; info?: UpdateInfo; disposition: UpdateErrorDisposition };

export type UpdateErrorDisposition = "retryable" | "recovery" | "manual";

export interface Updater {
  status: UpdateStatus;
  check: () => Promise<void>;
  /**
   * Automatic, throttled check that never supersedes an active operation.
   * Retryable failures stay quiet (logged, retried sooner); `check` reports them.
   */
  refresh: () => Promise<void>;
  /** Single-action update: download + verify + install + relaunch. */
  apply: (info: UpdateInfo) => void;
  openDownload: () => void;
  /** Discard a stuck previous update transaction so the next install can proceed. */
  abandonPending: () => Promise<void>;
  reset: () => void;
}

function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function classifyUpdateError(message: string): UpdateErrorDisposition {
  const low = message.toLowerCase();
  if (/pending update already exists|could not safely finish the previous update|handoff backup|awaiting startup health|discard the previous update|previous update is still completing/.test(low)) {
    return "recovery";
  }
  // An unknown install_layout is a deliberate migration boundary: retrying
  // never helps, only the full package from the download page does.
  if (/authorization failed|manual update required|pkexec|sudo apt install|unsupported install_layout/.test(low)) {
    return "manual";
  }
  return "retryable";
}

function updateError(message: string, info?: UpdateInfo): UpdateStatus {
  return { kind: "error", message, info, disposition: classifyUpdateError(message) };
}

const UpdaterContext = createContext<Updater | null>(null);

type UpdaterOperationKind = "idle" | "checking" | "ready" | "applying" | "abandoning";

interface UpdaterOperation {
  epoch: number;
  requestId: string;
  channel: "" | "stable" | "preview";
  expectedVersion: string;
  kind: UpdaterOperationKind;
}

let updaterRequestSequence = 0;
const updaterRequestPrefix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
export const UPDATE_CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000;
export const UPDATE_CHECK_STORAGE_KEY = "deepzora-updater-last-check-v1";
// A quiet automatic failure (offline at startup, slow release CDN) is retried
// sooner than the regular interval instead of waiting six hours (#117).
export const UPDATE_CHECK_FAILURE_RETRY_MS = 15 * 60 * 1000;
let processLastUpdateCheckAttemptMs: number | null = null;

function updateCheckStorage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    return null;
  }
}

function validUpdateCheckTimestamp(value: unknown, now: number): number | null {
  const timestamp = typeof value === "number" ? value : Number(value);
  return Number.isFinite(timestamp) && timestamp >= 0 && timestamp <= now ? timestamp : null;
}

function lastUpdateCheckAttempt(now: number): number | null {
  const memoryTimestamp = validUpdateCheckTimestamp(processLastUpdateCheckAttemptMs, now);
  if (memoryTimestamp === null) processLastUpdateCheckAttemptMs = null;

  const storage = updateCheckStorage();
  if (!storage) return memoryTimestamp;
  try {
    const raw = storage.getItem(UPDATE_CHECK_STORAGE_KEY);
    if (raw === null) return memoryTimestamp;
    const storedTimestamp = validUpdateCheckTimestamp(raw, now);
    if (storedTimestamp === null) {
      storage.removeItem(UPDATE_CHECK_STORAGE_KEY);
      return memoryTimestamp;
    }
    return memoryTimestamp === null ? storedTimestamp : Math.max(memoryTimestamp, storedTimestamp);
  } catch {
    return memoryTimestamp;
  }
}

function recordUpdateCheckAttempt(now = Date.now()): void {
  processLastUpdateCheckAttemptMs = now;
  try {
    updateCheckStorage()?.setItem(UPDATE_CHECK_STORAGE_KEY, String(now));
  } catch {
    // The process-local timestamp still prevents duplicate checks this run.
  }
}

// Backdating the attempt makes the next automatic check due after the shorter
// failure retry delay while still throttling focus/visibility bursts.
function recordFailedAutomaticCheck(now = Date.now()): void {
  recordUpdateCheckAttempt(Math.max(0, now - UPDATE_CHECK_INTERVAL_MS + UPDATE_CHECK_FAILURE_RETRY_MS));
}

function automaticUpdateCheckDue(now = Date.now()): boolean {
  const lastAttempt = lastUpdateCheckAttempt(now);
  return lastAttempt === null || now - lastAttempt >= UPDATE_CHECK_INTERVAL_MS;
}

export function __resetUpdaterCheckScheduleForTests(): void {
  processLastUpdateCheckAttemptMs = null;
  try {
    updateCheckStorage()?.removeItem(UPDATE_CHECK_STORAGE_KEY);
  } catch {
    // Tests also exercise storage-denied environments.
  }
}

function nextUpdaterRequestId(epoch: number): string {
  updaterRequestSequence += 1;
  return `web-${updaterRequestPrefix}-${epoch}-${updaterRequestSequence}`;
}

function normalizedChannel(channel: string): "stable" | "preview" {
  return channel === "preview" ? "preview" : "stable";
}

function isBusyOperation(kind: UpdaterOperationKind): boolean {
  return kind === "checking" || kind === "applying" || kind === "abandoning";
}

function useUpdaterInternal(): Updater {
  const [status, setStatus] = useState<UpdateStatus>({ kind: "idle" });
  const statusRef = useRef(status);
  statusRef.current = status;
  const operationRef = useRef<UpdaterOperation>({
    epoch: 0,
    requestId: "initial",
    channel: "",
    expectedVersion: "",
    kind: "idle",
  });

  const beginOperation = useCallback((
    channel: string,
    kind: UpdaterOperationKind,
    expectedVersion = "",
  ): UpdaterOperation => {
    const epoch = operationRef.current.epoch + 1;
    const next: UpdaterOperation = {
      epoch,
      requestId: nextUpdaterRequestId(epoch),
      channel: channel ? normalizedChannel(channel) : "",
      expectedVersion,
      kind,
    };
    operationRef.current = next;
    return next;
  }, []);

  const isCurrentOperation = useCallback((operation: UpdaterOperation): boolean => {
    const current = operationRef.current;
    return current.epoch === operation.epoch &&
      current.requestId === operation.requestId &&
      current.channel === operation.channel &&
      current.expectedVersion === operation.expectedVersion;
  }, []);

  const completeOperation = useCallback((operation: UpdaterOperation): void => {
    if (isCurrentOperation(operation)) {
      operationRef.current = { ...operationRef.current, kind: "ready" };
    }
  }, [isCurrentOperation]);

  // A single long-lived subscription advances the state machine through apply
  // phases. Channel and operation-kind checks prevent a superseded native call
  // from publishing into a newly selected channel.
  useEffect(() => {
    return onUpdaterProgress((p) => {
      const operation = operationRef.current;
      if (
        !p.requestId ||
        p.requestId !== operation.requestId ||
        !p.channel ||
        normalizedChannel(p.channel) !== operation.channel ||
        !p.version ||
        p.version !== operation.expectedVersion
      ) return;
      const accepted =
        operation.kind === "applying" &&
        (
          p.phase === "downloading" ||
          p.phase === "verifying" ||
          p.phase === "authorizing" ||
          p.phase === "installing" ||
          p.phase === "relaunching" ||
          p.phase === "done" ||
          p.phase === "error" ||
          p.phase === "cancelled"
        );
      if (!accepted) return;
      if (p.phase === "done" || p.phase === "error" || p.phase === "cancelled") {
        operationRef.current = { ...operation, kind: "ready" };
      }
      setStatus((cur) => {
        const info = "info" in cur ? cur.info : undefined;
        if (info && normalizedChannel(info.channel) !== operation.channel) return cur;
        switch (p.phase) {
          case "downloading":
            return info ? { kind: "downloading", received: p.received, total: p.total, info } : cur;
          case "verifying":
            return info ? { kind: "verifying", info } : cur;
          case "cancelled":
            return info ? { kind: "available", info } : { kind: "idle" };
          case "authorizing":
            return { kind: "authorizing", info };
          case "installing":
            return { kind: "installing", info };
          case "relaunching":
            return { kind: "relaunching", info };
          case "done":
            return { kind: "done" };
          case "error":
            return updateError(p.err ?? "update failed", info);
          default:
            return cur;
        }
      });
    });
  }, []);

  const runCheck = useCallback(async (trigger: "manual" | "automatic") => {
    // A newer check may supersede an in-flight check (and historically may
    // interrupt apply). Discard owns exclusive recovery work and must not be
    // epoch-stolen by Check/Retry while AbandonPendingUpdate is outstanding.
    if (operationRef.current.kind === "abandoning") return;
    recordUpdateCheckAttempt();
    const operation = beginOperation("stable", "checking");
    const before = statusRef.current;
    setStatus({ kind: "checking" });
    // Automatic checks run unprompted at startup and on focus, often before the
    // network is up. A retryable failure there is not actionable, so it stays
    // out of the UI and is retried soon; the user-initiated check (Settings,
    // banner Retry) still reports every failure. Recovery and manual-update
    // dispositions are always surfaced because they need the user (#117).
    const fail = (message: string, info?: UpdateInfo) => {
      const next = updateError(message, info);
      if (trigger === "automatic" && next.kind === "error" && next.disposition === "retryable") {
        console.warn(`[updater] automatic update check failed: ${message}`);
        recordFailedAutomaticCheck();
        setStatus(before.kind === "available" || before.kind === "upToDate" ? before : { kind: "idle" });
        return;
      }
      setStatus(next);
    };
    try {
      const info = await app.CheckUpdate("stable");
      if (!isCurrentOperation(operation)) return;
      if (!info) {
        completeOperation(operation);
        setStatus({ kind: "upToDate", current: "" });
        return;
      }
      const responseChannel = normalizedChannel(info.channel);
      if (operation.channel && responseChannel !== operation.channel) {
        completeOperation(operation);
        fail(`update check returned ${responseChannel} for requested ${operation.channel} channel`);
        return;
      }
      operation.channel = responseChannel;
      operation.expectedVersion = info.latest;
      operationRef.current = { ...operation, kind: "ready" };
      if (info.err) {
        fail(info.err, info);
        return;
      }
      if (!info.available) {
        setStatus({ kind: "upToDate", current: info.current });
        return;
      }
      setStatus({ kind: "available", info });
    } catch (e) {
      if (!isCurrentOperation(operation)) return;
      completeOperation(operation);
      fail(errMsg(e));
    }
  }, [beginOperation, completeOperation, isCurrentOperation]);

  const check = useCallback(() => runCheck("manual"), [runCheck]);

  const refresh = useCallback(async () => {
    if (isBusyOperation(operationRef.current.kind)) return;
    if (!automaticUpdateCheckDue()) return;
    await runCheck("automatic");
  }, [runCheck]);

  const apply = useCallback((info: UpdateInfo) => {
    const selectedChannel = normalizedChannel(info.channel);
    if (selectedChannel !== "stable") {
      setStatus(updateError("update check returned a retired release channel"));
      return;
    }
    const active = operationRef.current;
    if (isBusyOperation(active.kind) || (active.channel && active.channel !== selectedChannel)) return;
    if (!info.canSelfUpdate) {
      void app.OpenDownloadPage();
      return;
    }
    const operation = beginOperation(selectedChannel, "applying", info.latest);
    setStatus(
      info.requiresElevation || info.installMode === "deb"
        ? { kind: "authorizing", info }
        : { kind: "downloading", received: 0, total: info.assetSize, info },
    );
    void app.ApplyUpdateRequest(selectedChannel, info.latest, operation.requestId).catch((e) => {
      if (!isCurrentOperation(operation)) return;
      const message = errMsg(e);
      completeOperation(operation);
      setStatus(updateError(message, info));
    });
  }, [beginOperation, completeOperation, isCurrentOperation]);

  const openDownload = useCallback(() => {
    void app.OpenDownloadPage();
  }, []);

  const abandonPending = useCallback(async () => {
    const active = operationRef.current;
    if (isBusyOperation(active.kind)) return;
    // Publish busy UI immediately so Settings/Banner disable Retry/Check while
    // the discard promise is outstanding. Kind "abandoning" is distinct from
    // "checking" so a concurrent check cannot supersede the discard epoch.
    const operation = beginOperation(active.channel || "stable", "abandoning");
    setStatus({ kind: "checking" });
    try {
      if (typeof app.AbandonPendingUpdate === "function") {
        await app.AbandonPendingUpdate();
      }
      if (!isCurrentOperation(operation)) return;
      completeOperation(operation);
      setStatus({ kind: "idle" });
    } catch (e) {
      if (!isCurrentOperation(operation)) return;
      completeOperation(operation);
      setStatus(updateError(errMsg(e)));
    }
  }, [beginOperation, completeOperation, isCurrentOperation]);

  const reset = useCallback(() => {
    const epoch = operationRef.current.epoch + 1;
    operationRef.current = {
      epoch,
      requestId: nextUpdaterRequestId(epoch),
      channel: "",
      expectedVersion: "",
      kind: "idle",
    };
    setStatus({ kind: "idle" });
  }, []);

  return { status, check, refresh, apply, openDownload, abandonPending, reset };
}

export function UpdaterProvider({ children }: { children: ReactNode }) {
  const updater = useUpdaterInternal();
  return createElement(UpdaterContext.Provider, { value: updater, children });
}

export function useUpdater(): Updater {
  const updater = useContext(UpdaterContext);
  if (!updater) throw new Error("useUpdater must be used within an UpdaterProvider");
  return updater;
}
