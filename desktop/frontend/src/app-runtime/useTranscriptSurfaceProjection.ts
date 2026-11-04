import { useLayoutEffect } from "react";
import { useCommittedCommand } from "../lib/useCommittedCommand";
import type { useNavigationSurface } from "../lib/useNavigationSurface";
import type { HistoryLoadOutcome, HistoryLoadTrigger, Item, State } from "../lib/useController";
import type { SessionAvailability } from "../lib/sessionAvailability";
import type { NavigateToTurn, TurnNavigationTarget } from "../lib/historyTurnNavigation";

type NavigationSurfaceApi = ReturnType<typeof useNavigationSurface>;

export type TranscriptSurfaceProjectionInput = {
  hydrating: boolean;
  hydrateHistoryLoaded: boolean | undefined;
  hydratePlaceholderItems: Item[] | undefined;
  hydratePlaceholderActive: boolean;
  items: Item[];
  guidanceConsumed?: State["guidanceConsumed"];
  activeTabId: string | undefined;
  geometrySessionKey: string;
  controllerReady: boolean;
  availability: SessionAvailability;
  sessionActivity: boolean;
  sessionHasContent: boolean;
  commitRendered: NavigationSurfaceApi["commitRendered"];
  commitPaint: NavigationSurfaceApi["commitPaint"];
  commitSingleSurface: (tabId: string) => void;
  ports: {
    navigateToTurn?(tabId: string, target: TurnNavigationTarget, current: () => boolean): ReturnType<NavigateToTurn>;
    loadOlderHistory(tabId: string, targetTurn: number | undefined, trigger: HistoryLoadTrigger): Promise<HistoryLoadOutcome>;
    loadNewerHistory(tabId: string, latest: boolean, current?: () => boolean): Promise<HistoryLoadOutcome>;
    commitThenSend(tabId: string, displayText: string, submitText?: string): Promise<void>;
  };
};

/**
 * Owns the transcript surface projection: hydration placeholders, the
 * empty-session hero gate, the committed-surface commit effect (only
 * committed presentation may become a retained source), the visible
 * source-retained surface selection, surface paint receipts, the latest
 * consumed guidance entry and the transcript prompt/load-older commands.
 */
export function useTranscriptSurfaceProjection(input: TranscriptSurfaceProjectionInput) {
  const { activeTabId, ports } = input;
  const transcriptHydrating = input.hydrating && !input.hydrateHistoryLoaded;
  // Empty history owns the welcome layout; runtime readiness only gates sending.
  // A new/restored blank session can be readable while its controller starts.
  // History loading/errors and retained placeholders must still keep the main
  // surface visible so switching to an existing session never flashes a hero.
  const welcomeAvailable = input.availability.kind === "ready"
    || (input.availability.kind === "loading" && input.availability.source === "runtime");
  const emptyHero =
    welcomeAvailable &&
    !input.sessionActivity &&
    !input.sessionHasContent &&
    !transcriptHydrating &&
    !input.hydratePlaceholderActive;
  const transcriptItems = input.hydratePlaceholderActive ? input.hydratePlaceholderItems! : input.items;
  const handleLoadOlderHistory = useCommittedCommand((targetTurn?: number, trigger: HistoryLoadTrigger = "retry") => {
    return activeTabId ? ports.loadOlderHistory(activeTabId, targetTurn, trigger) : Promise.resolve("empty" as const);
  });
  const handleLoadNewerHistory = useCommittedCommand((latest = false, current?: () => boolean) => {
    return activeTabId ? ports.loadNewerHistory(activeTabId, latest, current) : Promise.resolve("empty" as const);
  });
  const handleNavigateToTurn = useCommittedCommand((target: TurnNavigationTarget, current: () => boolean) => {
    return activeTabId && ports.navigateToTurn ? ports.navigateToTurn(activeTabId, target, current) : Promise.resolve("cancelled" as const);
  });

  // Display items: backend history is authoritative after immediate commit.
  // rewindState only drives the undo banner, not optimistic truncation.
  const displayItems = transcriptItems;
  const committedSurfaceItems = displayItems;
  const committedGeometryKey = input.geometrySessionKey;
  // Only committed presentation can become a future retained source surface.
  // A suspended or abandoned render must never become navigation authority.
  const commitRendered = input.commitRendered;
  useLayoutEffect(() => {
    commitRendered({
      tabId: activeTabId,
      items: committedSurfaceItems,
      geometrySessionKey: committedGeometryKey,
    });
  }, [activeTabId, commitRendered, committedSurfaceItems, committedGeometryKey]);
  const visibleTranscriptItems = displayItems;
  const visibleTranscriptTabId = activeTabId;
  const visibleTranscriptGeometryKey = input.geometrySessionKey;
  const handleSurfacePaintReady = useCommittedCommand((token: string, outcome: "ready" | "degraded") => {
    const receipt = input.commitPaint(token, outcome);
    if (receipt) input.commitSingleSurface(receipt.targetTabId);
  });
  const latestGuidanceConsumed = input.guidanceConsumed ?? null;

  const handleTranscriptPrompt = useCommittedCommand((text: string, submitText = text) => {
    if (!activeTabId || !input.controllerReady) return;
    void ports.commitThenSend(activeTabId, text, submitText).catch((err) => {
      console.warn("Failed to submit transcript prompt", err);
    });
  });

  return {
    transcriptHydrating,
    emptyHero,
    visibleTranscriptItems,
    visibleTranscriptTabId,
    visibleTranscriptGeometryKey,
    handleLoadOlderHistory,
    handleLoadNewerHistory,
    handleNavigateToTurn,
    handleSurfacePaintReady,
    latestGuidanceConsumed,
    handleTranscriptPrompt,
  };
}
