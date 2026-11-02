import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useRef, useState } from "react";
import { app } from "./bridge";
import {
  beginKeyedResourceRequest,
  emptyKeyedResource,
  rejectKeyedResourceRequest,
  resolveKeyedResourceRequest,
} from "./keyedResource";
import type { WorkspaceChangesView } from "./types";

function errorMessage(error: unknown): string {
  if (typeof error === "object" && error && "message" in error) {
    return String((error as { message?: unknown }).message ?? error);
  }
  return String(error);
}

export function useWorkspaceChangesOwner(tabId: string, scopeKey: string, revision: number) {
  const [resource, setResource] = useState(() => emptyKeyedResource<WorkspaceChangesView>());
  const requestSeq = useRef(0);
  const key = `${scopeKey}\u0000working-tree`;
  const identity = useMemo(() => ({}), [tabId, scopeKey]);
  const currentIdentity = useRef<object | null>(null);
  const inFlight = useRef<{ identity: object; revision: number; promise: Promise<void> } | null>(null);
  useLayoutEffect(() => {
    currentIdentity.current = identity;
    return () => { currentIdentity.current = null; };
  }, [identity]);

  const load = useCallback(async (): Promise<void> => {
    if (!tabId || currentIdentity.current !== identity) return;
    const previous = inFlight.current;
    if (previous?.identity === identity) {
      if (previous.revision === revision) return previous.promise;
      await previous.promise;
      if (currentIdentity.current !== identity) return;
      return load();
    }
    const requestId = ++requestSeq.current;
    const requestKey = `${scopeKey}\u0000working-tree`;
    setResource((current) => beginKeyedResourceRequest(current, requestKey, requestId, revision));
    const request = { identity, revision, promise: Promise.resolve() };
    inFlight.current = request;
    request.promise = (async () => {
      try {
        const result = await app.WorkspaceChanges(tabId);
        if (currentIdentity.current !== identity || requestSeq.current !== requestId) return;
        const next = {
          files: Array.isArray(result?.files) ? result.files : [],
          gitAvailable: result?.gitAvailable !== false,
          gitErr: result?.gitErr,
          gitBranch: result?.gitBranch,
          added: result?.added,
          removed: result?.removed,
          incomplete: result?.incomplete,
        };
        setResource((current) => resolveKeyedResourceRequest(current, requestKey, requestId, next, revision));
      } catch (error) {
        if (currentIdentity.current !== identity || requestSeq.current !== requestId) return;
        setResource((current) => rejectKeyedResourceRequest(current, requestKey, requestId, errorMessage(error)));
      } finally {
        if (inFlight.current === request) inFlight.current = null;
      }
    })();
    return request.promise;
  }, [identity, revision, scopeKey, tabId]);

  const reset = useCallback(() => {
    requestSeq.current += 1;
    setResource(emptyKeyedResource());
  }, []);

  const current = resource.key === key;
  return {
    workspaceChanges: current ? resource.data : null,
    loadingWorkspaceChanges: current && resource.status === "refreshing",
    workspaceChangesErr: current ? resource.error : "",
    loadWorkspaceChanges: load,
    resetWorkspaceChanges: reset,
  };
}

type SharedWorkspaceChanges = {
  tabId: string;
  scopeKey: string;
  resource: ReturnType<typeof useWorkspaceChangesOwner>;
};

export const WorkspaceChangesContext = createContext<SharedWorkspaceChanges | null>(null);
const keepScopedResource = () => {};

/** The chat summary and every dock view read the same scoped snapshot. */
export function useWorkspaceChangesResource(tabId: string, scopeKey: string, revision: number) {
  const shared = useContext(WorkspaceChangesContext);
  const local = useWorkspaceChangesOwner(tabId, scopeKey, revision);
  if (shared?.tabId === tabId && shared.scopeKey === scopeKey) {
    // Changing the provider's scope already masks old data. A newly mounted
    // dock must not clear the summary's snapshot or its in-flight refresh.
    return { ...shared.resource, resetWorkspaceChanges: keepScopedResource };
  }
  return local;
}
