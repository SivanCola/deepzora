import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { HistoryWindowPage, HistoryWindowRequest, PersistentMessage, SubagentRunView } from "../generated/desktopContract.generated";
import { ErrorMessage } from "./ErrorMessage";

function messageText(message: PersistentMessage): string {
  const inline = message.inline;
  if (inline && typeof inline === "object" && "content" in inline && typeof inline.content === "string") return inline.content;
  return message.preview ?? "";
}

export function ConversationSubagents({ tabId, children, readOnly, onRefresh }: {
  tabId: string; children: SubagentRunView[]; readOnly: boolean; onRefresh: () => Promise<void>;
}) {
  const t = useT();
  const [selected, setSelected] = useState<SubagentRunView | null>(null);
  const [page, setPage] = useState<HistoryWindowPage | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [cancelling, setCancelling] = useState("");
  const [body, setBody] = useState<{ messageId: string; text: string; nextOffset: number } | null>(null);
  const sequence = useRef(0);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; sequence.current++; }; }, []);
  const read = useCallback(async (child: SubagentRunView, request: HistoryWindowRequest = { anchor: "newest" }) => {
    const ticket = ++sequence.current;
    setBody(null);
    setLoading(true); setError("");
    try {
      const result = await app.ReadSubagentHistoryForTab(tabId, child.ref, request);
      if (ticket !== sequence.current) return;
      if (result.status !== "ready") throw new Error(t("conversationStatus.historyPreparing"));
      setPage(result);
    } catch (failure) { if (ticket === sequence.current) setError(String(failure instanceof Error ? failure.message : failure)); }
    finally { if (ticket === sequence.current) setLoading(false); }
  }, [tabId, t]);
  const readBody = async (message: PersistentMessage, offset = 0) => {
    if (!selected) return;
    const ticket = ++sequence.current;
    setLoading(true); setError("");
    try {
      const chunk = await app.ReadSubagentMessageForTab(tabId, selected.ref, message.messageId, message.version, offset);
      if (ticket === sequence.current) setBody({ messageId: message.messageId, ...chunk });
    } catch (failure) { if (ticket === sequence.current) setError(String(failure instanceof Error ? failure.message : failure)); }
    finally { if (ticket === sequence.current) setLoading(false); }
  };
  const cancel = async (child: SubagentRunView) => {
    if (readOnly || cancelling) return;
    setCancelling(child.ref); setError("");
    try {
      const accepted = await app.CancelSubagentForTab(tabId, child.ref);
      if (!accepted) throw new Error(t("conversationStatus.notRunning"));
      if (mounted.current) await onRefresh();
    } catch (failure) { if (mounted.current) setError(String(failure instanceof Error ? failure.message : failure)); }
    finally { if (mounted.current) setCancelling(""); }
  };

  return <details open className="conversation-status-section">
    <summary>{t("conversationStatus.subagents")} <span>{children.length}</span></summary>
    <div className="conversation-status-section__body">
      {error && <p role="alert"><ErrorMessage error={error} /></p>}
      {selected ? <>
        <div className="conversation-status-actions">
          <button type="button" onClick={() => { sequence.current++; setSelected(null); setPage(null); setError(""); setCancelling(""); }}><ArrowLeft size={13} />{t("conversationStatus.back")}</button>
          <button type="button" disabled={loading} onClick={() => { void read(selected); }}><RefreshCw size={13} />{t("conversationStatus.refresh")}</button>
        </div>
        <strong>{selected.name}</strong><p className="conversation-status-note">{t("conversationStatus.childReadOnly")}</p>
        {loading && <p role="status">{t("conversationStatus.loading")}</p>}
        {page?.messages.map(message => <article className="conversation-status-message" key={message.messageId}>
          <small>{message.role === "user" ? t("conversationStatus.user") : message.role === "assistant" ? t("conversationStatus.assistant") : t("conversationStatus.tool")}</small>
          <pre>{body?.messageId === message.messageId ? body.text : messageText(message)}</pre>
          {!message.inline && message.contentRef && <>
            <small>{t("conversationStatus.messagePreview")}</small>
            <div className="conversation-status-actions">
              <button type="button" disabled={loading} onClick={() => { void readBody(message); }}>{t("conversationStatus.readMessage")}</button>
              {body?.messageId === message.messageId && body.nextOffset > 0 && <button type="button" disabled={loading} onClick={() => { void readBody(message, body.nextOffset); }}>{t("conversationStatus.readNext")}</button>}
            </div>
          </>}
        </article>)}
        {page && <div className="conversation-status-actions">
          <button type="button" disabled={loading || !page.hasOlder} onClick={() => { void read(selected, { anchor: "cursor", cursor: page.olderCursor, direction: "older" }); }}>{t("conversationStatus.older")}</button>
          <button type="button" disabled={loading || !page.hasNewer} onClick={() => { void read(selected, { anchor: "cursor", cursor: page.newerCursor, direction: "newer" }); }}>{t("conversationStatus.newer")}</button>
        </div>}
      </> : children.length ? children.map(child => <div className="conversation-status-item" key={child.ref}>
        <button type="button" className="conversation-status-row" onClick={() => { setSelected(child); setPage(null); void read(child); }}>
          <span>{child.name || child.kind}</span><small className="conversation-status-row__trailing">{t(child.status === "running" ? "conversationStatus.running" : child.status === "completed" ? "conversationStatus.completed" : child.status === "failed" ? "conversationStatus.failed" : "conversationStatus.stopped")}</small>
        </button>
        <div className="conversation-status-actions">
          <button type="button" onClick={() => { setSelected(child); setPage(null); void read(child); }}>{t("conversationStatus.openChild")}</button>
          {child.status === "running" && <button type="button" disabled={readOnly || Boolean(cancelling)} onClick={() => { void cancel(child); }}>{t("conversationStatus.cancel")}</button>}
        </div>
      </div>) : <p className="conversation-status-note">{t("conversationStatus.noSubagents")}</p>}
    </div>
  </details>;
}
