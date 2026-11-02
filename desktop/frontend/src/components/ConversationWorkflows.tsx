import { useT } from "../lib/i18n";
import type { HeartbeatTask } from "../custom/features/heartbeat/heartbeat.types";
import type { TabMeta } from "../lib/types";

export function ConversationWorkflows({ tasks, tabs, onOpenTopic, onOpenDirectory }: {
  tasks: HeartbeatTask[]; tabs: TabMeta[];
  onOpenTopic: (scope: string, root: string, topicId: string) => void;
  onOpenDirectory: () => void;
}) {
  const t = useT();
  return <details open className="conversation-status-section">
    <summary>{t("conversationStatus.workflows")} <span>{tasks.length}</span></summary>
    <div className="conversation-status-section__body">
      <button type="button" className="conversation-status-row" onClick={onOpenDirectory}>{t("conversationStatus.workflowDirectory")}</button>
      {tasks.length === 0 && <p className="conversation-status-note">{t("conversationStatus.noWorkflows")}</p>}
      {tasks.map(task => {
        const running = tabs.some(tab => tab.topicId === task.topicId && tab.scope === (task.scope || "global")
          && (task.scope !== "project" || tab.workspaceRoot === (task.workspaceRoot || "")) && (tab.running || tab.pendingPrompt || (tab.backgroundJobs ?? 0) > 0));
        return <details className="conversation-status-item" key={task.id}>
          <summary><span>{task.title}</span><small>{t(running ? "conversationStatus.running" : task.enabled ? "conversationStatus.scheduled" : "conversationStatus.paused")}</small></summary>
          <p className="conversation-status-note">{task.interval}</p>
          <pre className="conversation-status-plan">{task.prompt}</pre>
          {task.topicId && <button type="button" className="conversation-status-row" onClick={() => onOpenTopic(task.scope || "global", task.workspaceRoot || "", task.topicId!)}>{t("conversationStatus.runDetails")}</button>}
          <details>
            <summary>{t("conversationStatus.runHistory")}</summary>
            {[...(task.runHistory ?? [])].reverse().map((run, index) => <button type="button" key={`${run.at}:${run.topicId}:${index}`} className="conversation-status-row"
              disabled={!run.topicId} onClick={() => onOpenTopic(task.scope || "global", task.workspaceRoot || "", run.topicId)}>
              <span>{new Date(run.at).toLocaleString()}</span><small>{t("conversationStatus.triggered")}</small>
            </button>)}
            {!task.runHistory?.length && <p className="conversation-status-note">{t("conversationStatus.noRuns")}</p>}
          </details>
        </details>;
      })}
    </div>
  </details>;
}
