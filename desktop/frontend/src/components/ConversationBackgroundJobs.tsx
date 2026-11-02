import { useEffect, useRef, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { JobView } from "../lib/types";
import type { BackgroundJobInspection } from "../generated/desktopContract.generated";
import { ErrorMessage } from "./ErrorMessage";

export function ConversationBackgroundJobs({ tabId, jobs, readOnly, onCancelJob }: {
  tabId: string; jobs: JobView[]; readOnly: boolean; onCancelJob: (tabId: string, jobId: string) => Promise<boolean>;
}) {
  const t = useT();
  const [output, setOutput] = useState<BackgroundJobInspection | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const current = useRef(true);
  const request = useRef(0);
  useEffect(() => { current.current = true; return () => { current.current = false; request.current++; }; }, []);
  const read = async (id: string) => {
    const ticket = ++request.current;
    setBusy(id); setError("");
    try {
      const result = await app.InspectBackgroundJobForTab(tabId, id);
      if (current.current && ticket === request.current) setOutput(result);
    } catch (failure) { if (current.current && ticket === request.current) setError(String(failure instanceof Error ? failure.message : failure)); }
    finally { if (current.current && ticket === request.current) setBusy(""); }
  };
  const cancel = async (id: string) => {
    if (busy || readOnly) return;
    setBusy(id); setError("");
    try {
      if (!await onCancelJob(tabId, id)) throw new Error(t("conversationStatus.notRunning"));
    } catch (failure) { if (current.current) setError(String(failure instanceof Error ? failure.message : failure)); }
    finally { if (current.current) setBusy(""); }
  };
  return <details open className="conversation-status-section">
    <summary>{t("conversationStatus.terminals")} <span>{jobs.length}</span></summary>
    <div className="conversation-status-section__body">
      {jobs.length === 0 && <p className="conversation-status-note">{t("conversationStatus.noJobs")}</p>}
      {jobs.map(job => <div className="conversation-status-item" key={job.id}>
        <div className="conversation-status-row"><span>{job.label || job.id}</span><small>{t("conversationStatus.running")}</small></div>
        <div className="conversation-status-actions">
          <button type="button" disabled={Boolean(busy)} onClick={() => { void read(job.id); }}>{t("conversationStatus.output")}</button>
          <button type="button" disabled={readOnly || Boolean(busy)} onClick={() => { void cancel(job.id); }}>{t("conversationStatus.cancel")}</button>
        </div>
      </div>)}
      {error && <p role="alert"><ErrorMessage error={error} /></p>}
      {output && <div className="conversation-status-output">
        <strong>{output.label || output.id}</strong>
        <p className="conversation-status-note">{t("conversationStatus.outputTail")}</p>
        <pre>{output.output || t("conversationStatus.noOutput")}</pre>
        <button type="button" disabled={Boolean(busy)} onClick={() => { void read(output.id); }}>{t("conversationStatus.refresh")}</button>
      </div>}
    </div>
  </details>;
}
