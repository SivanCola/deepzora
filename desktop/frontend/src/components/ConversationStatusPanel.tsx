import { useRef, useState } from "react";
import { Activity, Check, ChevronDown, ListChecks, Minimize2, RefreshCw, Target } from "lucide-react";
import { useT } from "../lib/i18n";
import { useConversationStatusData } from "../lib/useConversationStatusData";
import { useWorkspaceChangesResource } from "../lib/useWorkspaceChangesResource";
import type { GoalLifecycleView, GoalRuntime, JobView, TabMeta } from "../lib/types";
import type { Todo } from "../lib/tools";
import { AnchoredPopover } from "./AnchoredPopover";
import { ConversationGitSection } from "./ConversationGitSection";
import { ConversationBackgroundJobs } from "./ConversationBackgroundJobs";
import { ConversationWorkflows } from "./ConversationWorkflows";
import { ConversationSubagents } from "./ConversationSubagents";
import { WorkspaceChangesBadge } from "./WorkspaceChangesBadge";
import { ErrorMessage } from "./ErrorMessage";

export type ConversationStatusPanelProps = {
  tabId: string; scopeKey: string; workspaceRoot: string; readOnly: boolean;
  running: boolean; goal: string; goalView?: GoalLifecycleView; goalRuntime?: GoalRuntime;
  todos: Todo[]; planText?: string; jobs: JobView[]; tabs: TabMeta[];
  onPauseGoal: () => void; onResumeGoal: () => void; onContinueTodos: () => void;
  onOpenChanges: () => void; onOpenAutomation: () => void;
  onOpenTopic: (scope: string, root: string, topicId: string) => void;
  onCancelJob: (tabId: string, jobId: string) => Promise<boolean>;
};

/** Compact summary first; the expanded panel holds scoped, actionable sections. */
export function ConversationStatusPanel(props: ConversationStatusPanelProps) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const { workspaceChanges: changes } = useWorkspaceChangesResource(props.tabId, props.scopeKey, 0);
  const data = useConversationStatusData(props.tabId, props.scopeKey, props.workspaceRoot, open);
  const current = props.todos.find(todo => todo.status === "in_progress");
  const done = props.todos.filter(todo => todo.status === "completed").length;
  const goal = props.goalView?.objective || props.goal;
  const goalResumable = props.goalView?.phase === "paused" || props.goalView?.phase === "blocked"
    || (props.goalView?.phase === "active" && props.goalView.activation === "disarmed");
  const hasChanges = Boolean(changes && ((changes.added ?? 0) + (changes.removed ?? 0) > 0
    || changes.files.some(file => file.sources.includes("git")) || changes.incomplete));
  const summary = props.jobs.length ? t("status.jobs", { n: props.jobs.length }) : current?.content || goal
    || (props.todos.length ? `${t("conversationStatus.plan")} ${done}/${props.todos.length}` : t("conversationStatus.title"));
  const close = () => { setOpen(false); triggerRef.current?.focus(); };

  return <div className="conversation-status" ref={anchorRef}>
    {hasChanges && !current && !goal && !props.jobs.length ? <WorkspaceChangesBadge tabId={props.tabId} scopeKey={props.scopeKey} expanded={open} buttonRef={triggerRef} onOpenChanges={() => setOpen(value => !value)} />
      : <div className="workspace-changes-bar"><button type="button" className="workspace-changes-badge conversation-status-trigger" ref={triggerRef}
          aria-label={t(open ? "conversationStatus.collapse" : "conversationStatus.expand")} aria-expanded={open}
          onClick={() => setOpen(value => !value)}>
          {props.jobs.length ? <Activity size={15} /> : current ? <ListChecks size={15} /> : goal ? <Target size={15} /> : <Activity size={15} />}
          <span>{summary}</span><ChevronDown size={13} />
        </button></div>}
    <AnchoredPopover open={open} anchorRef={anchorRef} onClose={close} className="conversation-status-popover" align="end" placement="bottom">
      <section className="conversation-status-panel" role="dialog" aria-label={t("conversationStatus.title")}>
        <header><strong>{t("conversationStatus.title")}</strong>
          <div className="conversation-status-panel__header-actions">
            <button type="button" className="conversation-status-icon" disabled={data.loading} aria-label={t("conversationStatus.refresh")} onClick={() => { void data.reload(); }}><RefreshCw size={14} /></button>
            <button type="button" className="conversation-status-icon" aria-label={t("conversationStatus.collapse")} onClick={close}><Minimize2 size={15} /></button>
          </div>
        </header>
        <div className="conversation-status-panel__sections">
          <ConversationGitSection tabId={props.tabId} scopeKey={props.scopeKey} workspaceRoot={props.workspaceRoot}
            readOnly={props.readOnly} running={props.running} onOpenChanges={() => { close(); props.onOpenChanges(); }} />
          {goal && <details open className="conversation-status-section">
            <summary>{t("conversationStatus.goal")}</summary>
            <div className="conversation-status-section__body">
              <p className="conversation-status-objective">{goal}</p>
              <p className="conversation-status-note">{t(props.goalView?.phase === "complete" ? "conversationStatus.completed" : goalResumable ? "conversationStatus.paused" : "conversationStatus.running")}</p>
              {props.goalView && <p className="conversation-status-note">{t("conversationStatus.rounds")}: {props.goalView.roundsStarted}{props.goalView.maxGoalRounds !== null ? ` / ${props.goalView.maxGoalRounds}` : ""}</p>}
              {props.goalRuntime && <p className="conversation-status-note">{t("conversationStatus.tokens")}: {props.goalRuntime.tokensUsed.toLocaleString()}{props.goalRuntime.tokensLimit > 0 ? ` / ${props.goalRuntime.tokensLimit.toLocaleString()}` : ""}</p>}
              {props.goalView?.blockedReason && <p className="conversation-status-note"><ErrorMessage error={props.goalView.blockedReason.message} /></p>}
              {props.goalView?.phase !== "complete" && <button type="button" className="conversation-status-row" disabled={props.readOnly}
                onClick={goalResumable ? props.onResumeGoal : props.onPauseGoal}>{t(goalResumable ? "composer.taskModeResumeGoal" : "composer.taskModePauseGoal")}</button>}
            </div>
          </details>}
          {(props.todos.length > 0 || props.planText) && <details open className="conversation-status-section">
            <summary>{t("conversationStatus.plan")} <span>{done}/{props.todos.length}</span></summary>
            <div className="conversation-status-section__body">
              {props.planText && <details><summary>{t("conversationStatus.planDetails")}</summary><pre className="conversation-status-plan">{props.planText}</pre></details>}
              <ol className="conversation-status-todos">{props.todos.map((todo, index) => <li key={`${index}:${todo.content}`} data-status={todo.status}>
                {todo.status === "completed" ? <Check size={13} /> : <span className={todo.status === "in_progress" ? "conversation-status-todo--active" : ""} />}
                <span>{todo.content}</span>
              </li>)}</ol>
              {!props.running && done < props.todos.length && <button type="button" className="conversation-status-row" disabled={props.readOnly} onClick={props.onContinueTodos}>{t("todo.continue")}</button>}
            </div>
          </details>}
          <ConversationBackgroundJobs tabId={props.tabId} jobs={props.jobs.filter(job => job.kind !== "task")}
            readOnly={props.readOnly} onCancelJob={props.onCancelJob} />
          <ConversationWorkflows tasks={data.workflows} tabs={props.tabs}
            onOpenTopic={(...args) => { close(); props.onOpenTopic(...args); }} onOpenDirectory={() => { close(); props.onOpenAutomation(); }} />
          <ConversationSubagents tabId={props.tabId} children={data.subagents} readOnly={props.readOnly} onRefresh={data.reload} />
          {data.loading && <p role="status" className="conversation-status-note">{t("conversationStatus.loading")}</p>}
          {data.errors.map((error, index) => <p role="alert" className="conversation-status-note" key={index}><ErrorMessage error={error} /></p>)}
        </div>
      </section>
    </AnchoredPopover>
  </div>;
}
