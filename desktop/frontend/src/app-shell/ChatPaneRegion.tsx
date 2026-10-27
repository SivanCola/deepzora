import type { ReactNode } from "react";
import { Transcript, type TranscriptProps } from "../components/Transcript";
import { SessionRecoveryBanner, SessionRecoveryPlaceholder } from "../components/SessionRecoveryBanner";
import { SessionLoadingIndicator } from "../components/SessionLoadingIndicator";
import { NoticePreviewPanel, noticePreviewMockEnabled } from "./NoticePreviewPanel";
import type { State } from "../lib/useController";
import type { Translator } from "../lib/i18n";
import type { ForkBlockReason } from "../lib/forkTargets";
import type { SessionAvailability } from "../lib/sessionAvailability";
import { orderedLocalSubmissions } from "../lib/localSubmissionState";


export type ChatPaneTranscriptInput = {
  state: State;
  items: TranscriptProps["items"];
  tabId: TranscriptProps["tabId"];
  geometrySessionKey: TranscriptProps["geometrySessionKey"];
  footerHeight: TranscriptProps["footerHeight"];
  invocationMetadata: TranscriptProps["invocationMetadata"];
  surfaceCommitToken: TranscriptProps["surfaceCommitToken"];
  liveStore: TranscriptProps["liveStore"];
  transcriptHydrating: boolean;
  readOnly: boolean;
  controllerReady: boolean;
  hydratePlaceholderActive: boolean;
  clearContextPending: boolean;
  emptyHero?: boolean;
  availability: SessionAvailability;
  rewind: {
    stateActive: boolean;
    committing: boolean;
  };
};

export type ChatPaneRegionProps = {
  t: Translator;
  /** Floating dock launcher card, mounted over the transcript's right edge. */
  launcher?: ReactNode;
  transcript: ChatPaneTranscriptInput;
  onRetryHistory: () => Promise<unknown>;
  commands: {
    onPrompt: TranscriptProps["onPrompt"];
    onFork: TranscriptProps["onFork"];
    onOpenTurnChanges?: TranscriptProps["onOpenTurnChanges"];
    onLoadOlderHistory: TranscriptProps["onLoadOlderHistory"];
    onLoadNewerHistory: TranscriptProps["onLoadNewerHistory"];
    onNavigateToTurn?: TranscriptProps["onNavigateToTurn"];
    onSurfacePaintReady: TranscriptProps["onSurfacePaintReady"];
  };
};

/**
 * The chat-pane main surface: IM/bot detail, notice preview mock or the
 * local transcript with its navigation-transition
 * wrapper and history-load error. Pure prop-driven; all ownership stays in
 * the caller's owners.
 */
export function ChatPaneRegion(props: ChatPaneRegionProps) {
  const { transcript, commands } = props;
  const { state, rewind } = transcript;
  // A fork entry reads persisted turn records, so it never waits for the session
  // to stop running, and a read-only source still forks: the child is written
  // from the source, never into it. It does wait for the surface it belongs to:
  // while the transcript hydrates or the source identity is switching, the
  // records on screen are not yet the ones a cut would address.
  const forkBlocked: ForkBlockReason | null = state.forkCreating ? "creating"
    : !transcript.controllerReady || transcript.transcriptHydrating || transcript.hydratePlaceholderActive
      ? "loading"
      : null;
  const noticePreview = noticePreviewMockEnabled();
  const localSubmissions = orderedLocalSubmissions(state);
  const recoveringEmpty = transcript.availability.kind !== "ready" && transcript.items.length === 0 && localSubmissions.length === 0
    && !state.live?.text && !state.live?.reasoning;
  const loading = transcript.availability.kind !== "error" && (transcript.transcriptHydrating
    || (transcript.availability.kind === "loading" && (transcript.availability.source !== "runtime" || recoveringEmpty)));
  return (
    <>
    {!noticePreview && <SessionRecoveryBanner key={transcript.geometrySessionKey ?? transcript.tabId}
      availability={transcript.availability} onRetry={props.onRetryHistory} />}
    <main className="main">
      {noticePreview ? (
        <NoticePreviewPanel />
      ) : (
        <>
          <div className="transcript-navigation-surface" aria-busy={loading}>
            {props.launcher}
            <div
              className="transcript-navigation-content"
              aria-hidden={transcript.emptyHero || undefined}
              ref={(node) => {
                if (!node) return;
                (node as HTMLElement & { inert?: boolean }).inert = Boolean(transcript.emptyHero);
              }}
            >
              {recoveringEmpty ? <SessionRecoveryPlaceholder availability={transcript.availability} /> : <Transcript
                items={transcript.items}
                localSubmissions={localSubmissions}
                localSubmissionSendRevision={state.localSubmissionSendRevision}
                visibleSubmissionHandoffs={state.visibleSubmissionHandoffs}
                live={state.live}
                liveStore={transcript.liveStore}
                tabId={transcript.tabId}
                geometrySessionKey={transcript.geometrySessionKey}
                footerHeight={transcript.footerHeight}
                onPrompt={commands.onPrompt}
                onFork={commands.onFork}
                onOpenTurnChanges={commands.onOpenTurnChanges}
                forkTargets={state.forkTargets}
                forkBlocked={forkBlocked}
                running={(state.running && !state.runtimeStateSnapshot?.maintenance) || rewind.committing}
                turnStartAt={state.turnStartAt}
                hydrating={transcript.transcriptHydrating}
                showLoadingFeedback={false}
                hasOlderHistory={state.historyHasOlder && !rewind.stateActive}
                hasNewerHistory={state.historyHasNewer && !rewind.stateActive}
                historyStartTurn={state.historyStartTurn}
                historyEndTurn={state.historyEndTurn}
                totalTurns={state.historyTotalTurns}
                loadingOlderHistory={state.historyOlderLoading}
                olderHistoryError={state.historyOlderError}
                loadingNewerHistory={state.historyNewerLoading}
                newerHistoryError={state.historyNewerError}
                onLoadOlderHistory={commands.onLoadOlderHistory}
                onLoadNewerHistory={commands.onLoadNewerHistory}
                onNavigateToTurn={commands.onNavigateToTurn}
                invocationMetadata={transcript.invocationMetadata}
                surfaceCommitToken={transcript.surfaceCommitToken}
                onSurfacePaintReady={commands.onSurfacePaintReady}
              />}
            </div>
            <SessionLoadingIndicator active={loading}
              identity={transcript.geometrySessionKey ?? transcript.tabId ?? "local"}
              source={transcript.availability.source} />
          </div>
        </>
      )}
    </main>
    </>
  );
}
