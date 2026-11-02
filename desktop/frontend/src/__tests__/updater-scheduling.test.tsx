import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { UpdateBanner } from "../components/UpdateBanner";
import { LocaleProvider } from "../lib/i18n";
import {
  __resetUpdaterCheckScheduleForTests,
  UPDATE_CHECK_FAILURE_RETRY_MS,
  UPDATE_CHECK_INTERVAL_MS,
  UPDATE_CHECK_STORAGE_KEY,
  UpdaterProvider,
  useUpdater,
} from "../lib/useUpdater";
import { installDesktopHostStub } from "./desktopHostStub";

const dom = new JSDOM("<!doctype html><div id='root'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  Node: dom.window.Node,
  Element: dom.window.Element,
  HTMLElement: dom.window.HTMLElement,
  Event: dom.window.Event,
  MouseEvent: dom.window.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
});

let now = 1_800_000_000_000;
const originalDateNow = Date.now;
Date.now = () => now;

let checkCalls = 0;
let checkErr = "";
const manifestTimeout = 'update: fetch manifest: Get "https://github.com/SivanCola/deepzora/releases/download/v0.0.2/latest.json": context deadline exceeded';
const desktopStub = installDesktopHostStub({
  CheckUpdate: async () => {
    checkCalls += 1;
    return {
      ...(checkErr ? { err: checkErr } : {}),
      available: false,
      current: "v1.2.3",
      latest: "v1.2.3",
      notes: "",
      channel: "stable",
      canSelfUpdate: true,
      manualOnly: false,
      installMode: "portable",
      requiresElevation: false,
      downloaded: false,
      downloadUrl: "https://example.invalid/download",
      assetSize: 0,
    };
  },
});

function ManualCheck() {
  const updater = useUpdater();
  return <button id="manual-check" onClick={() => void updater.check()}>Check</button>;
}

function AutomaticCheck() {
  const updater = useUpdater();
  useEffect(() => { void updater.refresh(); }, [updater.refresh]);
  return null;
}

async function flush() {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

const root = createRoot(document.getElementById("root")!);
try {
  __resetUpdaterCheckScheduleForTests();
  await act(async () => {
    root.render(<LocaleProvider><UpdaterProvider><UpdateBanner enabled /><ManualCheck /></UpdaterProvider></LocaleProvider>);
    await flush();
  });
  assert.equal(checkCalls, 1, "first enabled mount performs one automatic check");

  now += UPDATE_CHECK_INTERVAL_MS - 1;
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
    document.dispatchEvent(new Event("visibilitychange"));
    await flush();
  });
  assert.equal(checkCalls, 1, "focus and visibility events stay throttled before six hours");

  now += 1;
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
    document.dispatchEvent(new Event("visibilitychange"));
    await flush();
  });
  assert.equal(checkCalls, 2, "simultaneous due events produce one check after six hours");

  await act(async () => root.render(<LocaleProvider><UpdaterProvider><AutomaticCheck /><ManualCheck /></UpdaterProvider></LocaleProvider>));
  await act(async () => { await flush(); });
  assert.equal(checkCalls, 2, "stored attempt survives provider remounts");

  await act(async () => {
    (document.getElementById("manual-check") as HTMLButtonElement).click();
    await flush();
  });
  assert.equal(checkCalls, 3, "manual checks bypass the automatic throttle");
  assert.equal(window.localStorage.getItem(UPDATE_CHECK_STORAGE_KEY), String(now), "manual check resets the automatic schedule");

  now += UPDATE_CHECK_INTERVAL_MS - 1;
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
    await flush();
  });
  assert.equal(checkCalls, 3, "manual check suppresses automatic checks for the next six hours");

  await act(async () => root.render(<LocaleProvider><UpdaterProvider><UpdateBanner enabled={false} /></UpdaterProvider></LocaleProvider>));
  now += UPDATE_CHECK_INTERVAL_MS;
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
    document.dispatchEvent(new Event("visibilitychange"));
    await flush();
  });
  assert.equal(checkCalls, 3, "disabled automatic checks install no startup, focus, or visibility work");

  __resetUpdaterCheckScheduleForTests();
  window.localStorage.setItem(UPDATE_CHECK_STORAGE_KEY, String(now + UPDATE_CHECK_INTERVAL_MS));
  await act(async () => root.render(<LocaleProvider><UpdaterProvider><AutomaticCheck /></UpdaterProvider></LocaleProvider>));
  await act(async () => { await flush(); });
  assert.equal(checkCalls, 4, "a future timestamp is discarded and rebuilt by an immediate check");

  const localStorageDescriptor = Object.getOwnPropertyDescriptor(window, "localStorage");
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    get() { throw new Error("storage denied"); },
  });
  __resetUpdaterCheckScheduleForTests();
  await act(async () => root.render(<LocaleProvider><UpdaterProvider><UpdateBanner enabled /></UpdaterProvider></LocaleProvider>));
  await act(async () => { await flush(); });
  assert.equal(checkCalls, 5, "storage denial still allows the first process-local check");
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
    document.dispatchEvent(new Event("visibilitychange"));
    await flush();
  });
  assert.equal(checkCalls, 5, "process-local fallback throttles duplicate checks when storage is unavailable");
  if (localStorageDescriptor) Object.defineProperty(window, "localStorage", localStorageDescriptor);

  // #117: an automatic check that times out (e.g. first launch after an update,
  // before the network is ready) must not surface an error banner.
  const originalWarn = console.warn;
  const warnings: string[] = [];
  console.warn = (...args: unknown[]) => { warnings.push(args.map(String).join(" ")); };
  try {
    __resetUpdaterCheckScheduleForTests();
    checkErr = manifestTimeout;
    const callsBeforeFailure = checkCalls;
    await act(async () => root.render(<LocaleProvider><UpdaterProvider key="offline"><UpdateBanner enabled /><ManualCheck /></UpdaterProvider></LocaleProvider>));
    await act(async () => { await flush(); });
    assert.equal(checkCalls, callsBeforeFailure + 1, "startup performs the automatic check");
    assert.equal(document.querySelector(".banner--error"), null, "a transient automatic failure shows no error banner");
    assert.ok(warnings.some((w) => w.includes("context deadline exceeded")), "the automatic failure is logged");

    now += UPDATE_CHECK_FAILURE_RETRY_MS - 1;
    await act(async () => { window.dispatchEvent(new Event("focus")); await flush(); });
    assert.equal(checkCalls, callsBeforeFailure + 1, "a failed automatic check is not retried on every focus");

    now += 1;
    checkErr = "";
    await act(async () => { window.dispatchEvent(new Event("focus")); await flush(); });
    assert.equal(checkCalls, callsBeforeFailure + 2, "a failed automatic check is retried after the short failure delay");
    assert.equal(document.querySelector(".banner--error"), null, "a successful retry leaves no banner");

    checkErr = manifestTimeout;
    await act(async () => {
      (document.getElementById("manual-check") as HTMLButtonElement).click();
      await flush();
    });
    assert.ok(document.querySelector(".banner--error"), "a user-initiated check still reports the failure");

    checkErr = 'update: fetch manifest: https://example.invalid/latest.json: platforms windows-amd64 asset: unsupported install_layout "x" (keeping current version)';
    __resetUpdaterCheckScheduleForTests();
    await act(async () => root.render(<LocaleProvider><UpdaterProvider key="fresh"><UpdateBanner enabled /></UpdaterProvider></LocaleProvider>));
    await act(async () => { await flush(); });
    assert.ok(document.querySelector(".banner--error"), "automatic checks still surface failures that need the user");
  } finally {
    console.warn = originalWarn;
    checkErr = "";
  }

  console.log("PASS updater six-hour scheduling, persistence, manual bypass, disable, storage recovery, and quiet automatic failures");
} finally {
  await act(async () => root.unmount());
  desktopStub.uninstall();
  Date.now = originalDateNow;
  dom.window.close();
}
