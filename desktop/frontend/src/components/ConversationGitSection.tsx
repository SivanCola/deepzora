import { useEffect, useRef, useState } from "react";
import { GitBranch, Plus, RefreshCw } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { useBranchSwitcher } from "../lib/useBranchSwitcher";
import { useWorkspaceChangesResource } from "../lib/useWorkspaceChangesResource";
import { markWorkspaceRefresh } from "../lib/workspaceRefreshStore";
import { ErrorMessage } from "./ErrorMessage";

export function ConversationGitSection({ tabId, scopeKey, workspaceRoot, readOnly, running, onOpenChanges }: {
  tabId: string; scopeKey: string; workspaceRoot: string; readOnly: boolean; running: boolean; onOpenChanges: () => void;
}) {
  const t = useT();
  const rootRef = useRef<HTMLDivElement>(null);
  const { workspaceChanges: changes, workspaceChangesErr, loadWorkspaceChanges } = useWorkspaceChangesResource(tabId, scopeKey, 0);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState("");
  const [result, setResult] = useState<{ error: boolean; text: string } | null>(null);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  // The section is mounted under a session-keyed panel. Capture its exact
  // workspace in every bridge call; late UI updates are fenced by the parent.
  const branch = useBranchSwitcher({ tabId, scopeKey, workspaceRoot, gitBranch: changes?.gitBranch,
    rootRef, onBranchChanged: () => { void loadWorkspaceChanges(); } });
  const disabled = readOnly || running || Boolean(busy || branch.switchingBranch);
  const run = async (action: string) => {
    if (disabled) return;
    setBusy(action); setResult(null);
    try {
      const text = await app.RunWorkspaceGitActionForTab(tabId, workspaceRoot, action, message);
      if (!mounted.current) return;
      if (action === "commit") setMessage("");
      setResult({ error: false, text: text || t("conversationStatus.actionDone") });
      markWorkspaceRefresh(tabId, scopeKey);
      await loadWorkspaceChanges();
    } catch (error) {
      if (mounted.current) setResult({ error: true, text: String(error instanceof Error ? error.message : error) });
    } finally { if (mounted.current) setBusy(""); }
  };
  if (!changes?.gitAvailable && !changes?.gitErr && !workspaceChangesErr) return null;
  return <details open className="conversation-status-section">
    <summary>{t("conversationStatus.environment")}</summary>
    <div className="conversation-status-section__body" ref={rootRef}>
      <button type="button" className="conversation-status-row" onClick={onOpenChanges}>
        <span>{t("workspace.changeSummary")}</span>
        <span className="conversation-status-row__trailing">
          {(changes?.incomplete || workspaceChangesErr || changes?.gitErr) && "~ "}
          <span className="workspace-changes-badge__added">+{(changes?.added ?? 0).toLocaleString()}</span>{" "}
          <span className="workspace-changes-badge__removed">−{(changes?.removed ?? 0).toLocaleString()}</span>
        </span>
      </button>
      {changes?.gitAvailable && <>
        <button type="button" className="conversation-status-row" disabled={disabled} onClick={branch.toggleBranchMenu} aria-expanded={branch.branchMenuOpen}>
          <GitBranch size={14} /><span>{branch.activeBranch || t("status.gitBranchTitle")}</span>
        </button>
        {branch.branchMenuOpen && <div className="conversation-status-branch-menu">
          <input ref={branch.branchSearchRef} value={branch.branchQuery} aria-label={t("rightDock.branchSearchPlaceholder")}
            placeholder={t("rightDock.branchSearchPlaceholder")} onChange={event => branch.setBranchQuery(event.target.value)} />
          {branch.branchesLoading && <p>{t("rightDock.branchMenuLoading")}</p>}
          {branch.filteredBranches.map(name => <button type="button" className="conversation-status-row" key={name}
            disabled={disabled || name === branch.activeBranch} onClick={() => { void branch.checkoutBranch(name); }}>{name}</button>)}
          <button type="button" className="conversation-status-row" disabled={disabled || !branch.canCreate}
            onClick={() => { void branch.createBranch(branch.trimmedQuery); }}><Plus size={14} />{t("rightDock.branchCreate")} {branch.trimmedQuery}</button>
          {(branch.branchesErr || branch.branchSwitchErr) && <p role="alert"><ErrorMessage error={branch.branchesErr || branch.branchSwitchErr} /></p>}
        </div>}
        <details className="conversation-status-git-actions">
          <summary>{t("conversationStatus.gitActions")}</summary>
          <div className="conversation-status-actions">
            <button type="button" disabled={disabled} onClick={() => { void run("stage"); }}>{t("conversationStatus.stage")}</button>
            <button type="button" disabled={disabled} onClick={() => { void run("fetch"); }}>{t("conversationStatus.fetch")}</button>
            <button type="button" disabled={disabled} onClick={() => { void run("pull"); }}>{t("conversationStatus.pull")}</button>
            <button type="button" disabled={disabled} onClick={() => { void run("push"); }}>{t("conversationStatus.push")}</button>
          </div>
          <form onSubmit={event => { event.preventDefault(); void run("commit"); }}>
            <input value={message} disabled={disabled} placeholder={t("conversationStatus.commitMessage")}
              aria-label={t("conversationStatus.commitMessage")} onChange={event => setMessage(event.target.value)} />
            <button type="submit" disabled={disabled || !message.trim()}>{t("conversationStatus.commit")}</button>
          </form>
          {running && <p className="conversation-status-note">{t("conversationStatus.gitBusy")}</p>}
          {result && <p role={result.error ? "alert" : "status"} className="conversation-status-note">{result.error ? <ErrorMessage error={result.text} /> : result.text}</p>}
        </details>
      </>}
      {(workspaceChangesErr || changes?.gitErr) && <p role="alert" className="conversation-status-note"><ErrorMessage error={workspaceChangesErr || changes?.gitErr} /></p>}
      <button type="button" className="conversation-status-row" onClick={() => { void loadWorkspaceChanges(); }}><RefreshCw size={13} />{t("conversationStatus.refresh")}</button>
    </div>
  </details>;
}
