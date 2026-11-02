import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { app, onEvent } from "./bridge";
import { createWorkspaceRefreshScheduler } from "./workspaceRefreshScheduler";
import { heartbeatListTasks } from "../custom/features/heartbeat/heartbeat.bridge";
import type { HeartbeatTask } from "../custom/features/heartbeat/heartbeat.types";
import type { SubagentRunView } from "../generated/desktopContract.generated";

export function useConversationStatusData(tabId: string, scopeKey: string, workspaceRoot: string, open: boolean) {
  const identity = useMemo(() => ({}), [tabId, scopeKey, workspaceRoot]);
  const current = useRef<object | null>(null);
  const sequence = useRef(0);
  const [snapshot, setSnapshot] = useState<{ identity: object; subagents: SubagentRunView[]; workflows: HeartbeatTask[]; errors: string[] } | null>(null);
  const [loading, setLoading] = useState(false);
  const inFlight = useRef<Promise<void> | null>(null);
  useLayoutEffect(() => {
    current.current = identity;
    return () => { current.current = null; sequence.current++; };
  }, [identity]);

  const reload = useCallback(async () => {
    if (!tabId || current.current !== identity) return;
    // Requests for a previous identity may complete independently; responses
    // are fenced before publication, never reused for the replacement tab.
    const request = ++sequence.current;
    setLoading(true);
    const result = await Promise.allSettled([app.ListSubagentRunsForTab(tabId), heartbeatListTasks()]);
    if (current.current !== identity || sequence.current !== request) return;
    const [children, automations] = result;
    const errors = result.flatMap(entry => entry.status === "rejected" ? [String(entry.reason?.message ?? entry.reason)] : []);
    setSnapshot(previous => ({ identity,
      subagents: children.status === "fulfilled" ? children.value ?? [] : previous?.identity === identity ? previous.subagents : [],
      workflows: automations.status === "fulfilled"
        ? automations.value.filter(task => task.scope !== "project" || task.workspaceRoot === workspaceRoot)
        : previous?.identity === identity ? previous.workflows : [],
      errors,
    }));
    setLoading(false);
  }, [identity, tabId, workspaceRoot]);

  useEffect(() => {
    if (!open || !tabId) return;
    const scheduler = createWorkspaceRefreshScheduler(300);
    let live = true;
    const refresh = async () => {
      if (!live || document.visibilityState === "hidden") return;
      if (inFlight.current) await inFlight.current;
      if (!live) return;
      const request = reload(); inFlight.current = request;
      try { await request; } finally { if (inFlight.current === request) inFlight.current = null; }
    };
    void refresh();
    const stop = onEvent(event => {
      // Automation runs normally live in another tab. Their lifecycle can
      // replace topic IDs and append history for workflows visible here.
      const lifecycle = event.kind === "turn_started" || event.kind === "turn_done" || event.kind === "notice";
      const localProgress = event.tabId === tabId && (event.kind === "tool_result"
        || (event.kind === "tool_progress" && event.tool?.name.endsWith("status")));
      if (lifecycle || localProgress) scheduler.trigger(refresh);
    });
    const onVisible = () => { if (document.visibilityState !== "hidden") scheduler.trigger(refresh); };
    document.addEventListener("visibilitychange", onVisible);
    return () => { live = false; scheduler.cancel(); stop(); document.removeEventListener("visibilitychange", onVisible); };
  }, [open, reload, tabId]);

  const data = snapshot?.identity === identity ? snapshot : null;
  return { subagents: data?.subagents ?? [], workflows: data?.workflows ?? [], errors: data?.errors ?? [], loading, reload };
}
