import { useLayoutEffect, useRef, type Dispatch, type SetStateAction } from "react";
import type { Translator } from "../lib/i18n";
import type { useToast } from "../lib/toast";
import type { SessionMeta } from "../lib/types";
import { refreshHistoryProjection, type HistoryViewState } from "./historyViewProjection";
import { useCommittedCommand } from "../lib/useCommittedCommand";
import { enqueueNavigationRequest, type NavigationCoalescingRefs } from "../lib/openTopicCoalescing";
import { useResourceOperations, type SessionResource, type SessionOperationAuthority } from "./useResourceOperations";
import type { DesktopNavigationCapture, DesktopNavigationIntent, DesktopNavigationPorts, NavigationNotice } from "./desktopNavigationOwner";

type QueueInput = { capture: DesktopNavigationCapture; authority: SessionOperationAuthority; result: { error?: unknown } };
let executeNavigation: typeof import("./desktopNavigationOwner").executeDesktopNavigation | undefined;
async function runQueuedRequest(request: QueueInput) {
  try {
    executeNavigation ??= (await import("./desktopNavigationOwner")).executeDesktopNavigation;
    await executeNavigation(request.capture, request.authority);
  }
  catch (error) { request.result.error = error; }
}
async function executeQueuedNavigation(input: { capture: DesktopNavigationCapture; queue: NavigationCoalescingRefs<QueueInput> }, authority: SessionOperationAuthority) {
  const result: QueueInput["result"] = {};
  await enqueueNavigationRequest(input.queue, { capture: input.capture, authority, result }, runQueuedRequest);
  if (result.error) throw result.error;
}

/** Owns the existing last-click-wins queue; no App render or view model is queued. */
export function useDesktopNavigation(input: {
  visible: SessionResource;
  ports: Omit<DesktopNavigationPorts, "reveal" | "projectChanged" | "closeHistory" | "notice" | "applyHistorySessions">;
  setTabRevealSignal: Dispatch<SetStateAction<number>>;
  setProjectRevision: Dispatch<SetStateAction<number>>;
  setHistory: Dispatch<SetStateAction<HistoryViewState | null>>;
  t: Translator;
  showToast: ReturnType<typeof useToast>["showToast"];
  noteIntent(): number;
  beginSurface(seq: number): void;
  settleSurface(seq: number): void;
  showChat(): void;
}) {
  const operations = useResourceOperations({ visible: input.visible });
  const reveal = useCommittedCommand(() => { input.setTabRevealSignal(value => value + 1); });
  const projectChanged = useCommittedCommand(() => input.setProjectRevision(value => value + 1));
  const closeHistory = useCommittedCommand(() => input.setHistory(null));
  const applyHistorySessions = useCommittedCommand((sessions: SessionMeta[]) => input.setHistory(current => refreshHistoryProjection(current, sessions)));
  const notice = useCommittedCommand((notice: NavigationNotice) => {
    input.showToast("key" in notice ? input.t(notice.key, notice.params) : notice.message, notice.tone, { durationMs: notice.durationMs });
  });
  const queueRef = useRef<NavigationCoalescingRefs<QueueInput> | null>(null);
  if (!queueRef.current) queueRef.current = { seqRef: { current: 0 }, runningRef: { current: false }, pendingRef: { current: null } };
  const queue = queueRef.current;
  const settle = useCommittedCommand(input.settleSurface);
  const executeWithIntent = useCommittedCommand(async (intent: DesktopNavigationIntent, navigationIntentSeq: number) => {
    input.beginSurface(navigationIntentSeq);
    try {
      return await operations({ kind: "application" }, "navigation", {
        queue, capture: { intent, navigationIntentSeq,
          ports: { ...input.ports, reveal, projectChanged, closeHistory, notice, applyHistorySessions } },
      }, executeQueuedNavigation);
    } finally { settle(navigationIntentSeq); }
  });
  const enqueueNavigationWithIntent = useCommittedCommand(async (intent: DesktopNavigationIntent, seq: number): Promise<void> => { await executeWithIntent(intent, seq); });
  const enqueueNavigation = useCommittedCommand((intent: DesktopNavigationIntent) => {
    input.showChat();
    return enqueueNavigationWithIntent(intent, input.noteIntent());
  });
  useLayoutEffect(() => () => {
    queue.seqRef.current++;
    queue.pendingRef.current?.resolve();
    queue.pendingRef.current = null;
  }, [queue]);
  return { enqueueNavigation, enqueueNavigationWithIntent };
}
