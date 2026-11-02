import { useEffect, useRef, useState, type ReactNode } from "react";
import { WorkspaceChangesContext, useWorkspaceChangesOwner } from "../lib/useWorkspaceChangesResource";
import { useWorkspaceRefresh } from "../lib/workspaceRefreshStore";
import { workspaceRefreshFallbackSequence } from "../lib/workspaceRefreshInvalidation";
import { createWorkspaceRefreshScheduler } from "../lib/workspaceRefreshScheduler";

/** One read owner for the summary and dock; refreshes use the existing watcher. */
export function WorkspaceChangesProvider({ tabId, scopeKey, enabled, running, children }: {
  tabId: string;
  scopeKey: string;
  enabled: boolean;
  running: boolean;
  children: ReactNode;
}) {
  const [foreground, setForeground] = useState(() => document.visibilityState !== "hidden");
  const active = enabled && foreground && Boolean(tabId);
  const refresh = useWorkspaceRefresh(tabId, scopeKey, active);
  const resource = useWorkspaceChangesOwner(tabId, scopeKey, refresh.revisions.workingTree);
  const scheduler = useRef<ReturnType<typeof createWorkspaceRefreshScheduler> | null>(null);
  if (!scheduler.current) scheduler.current = createWorkspaceRefreshScheduler(300);
  const fallbackSequence = workspaceRefreshFallbackSequence(refresh);

  useEffect(() => {
    const onVisibility = () => setForeground(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, []);

  useEffect(() => {
    const owner = scheduler.current!;
    if (active) owner.trigger(resource.loadWorkspaceChanges);
    else owner.cancel();
    return () => owner.cancel();
  }, [active, fallbackSequence, refresh.revisions.gitMeta, refresh.revisions.session, resource.loadWorkspaceChanges, running]);

  return <WorkspaceChangesContext.Provider value={{ tabId, scopeKey, resource }}>
    {children}
  </WorkspaceChangesContext.Provider>;
}
