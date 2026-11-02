import { createContext, useCallback, useContext, useEffect, useId, useLayoutEffect, useRef, useState, type MutableRefObject, type ReactNode } from "react";
import { createPortal, flushSync } from "react-dom";
import { ArrowLeft, CircleAlert, CircleCheck, Info, Loader2 } from "lucide-react";
import { useT } from "../lib/i18n";
import { ErrorMessage } from "./ErrorMessage";
import { useToast, type ToastContextValue } from "../lib/toast";

export function SettingsFeedback({ kind, children, onRetry }: {
  kind: "loading" | "empty" | "error" | "success" | "info";
  children?: ReactNode; onRetry?: () => void;
}) {
  const t = useT();
  const Icon = kind === "loading" ? Loader2 : kind === "error" ? CircleAlert : kind === "success" ? CircleCheck : Info;
  return <div className={`settings-feedback settings-feedback--${kind}`} role={kind === "error" ? "alert" : "status"} aria-busy={kind === "loading" || undefined}>
    <Icon size={16} aria-hidden="true" className={kind === "loading" ? "spin" : undefined} />
    <div>{kind === "error" && typeof children === "string" ? <ErrorMessage error={children} /> : children ?? t(kind === "loading" ? "settings.loading" : "common.none")}</div>
    {onRetry && <button type="button" className="btn btn--small" onClick={onRetry}>{t("common.retry")}</button>}
  </div>;
}

export function SettingsSubpageHeader({ title, description, onBack, backLabel, actions, className = "", backClassName = "", mode = "manual" }: {
  title: ReactNode; description?: ReactNode; onBack: () => void; backLabel?: string;
  actions?: ReactNode; className?: string; backClassName?: string; mode?: "manual" | "auto" | "action" | "preview";
}) {
  const t = useT();
  const { requestLeave } = useSettingsExperience();
  return <header className={`settings-subpage-header ${className}`} data-settings-subpage="" data-save-mode={mode}>
    <button type="button" className={`settings-back-button ${backClassName}`} onClick={() => requestLeave(onBack)}><ArrowLeft size={16} aria-hidden="true" />{backLabel ?? t("settings.backToParent")}</button>
    <div className="settings-subpage-header__row"><h2>{title}</h2><div className="settings-toolbar__actions">{actions}</div></div>
    {description && <p>{description}</p>}
    {mode !== "action" && <small>{t(mode === "auto" ? "settings.saveAutomatically" : mode === "preview" ? "settings.previewSaveHint" : "settings.saveExplicitly")}</small>}
  </header>;
}

type Draft = {
  label: string; dirty: boolean; busy?: boolean; canSave?: boolean;
  /** Child editors apply to their parent before the parent persists. */
  order?: number;
  save?: () => Promise<boolean>; discard?: () => void;
};
type LeaveRequest = { action: () => void; ids?: string[]; cancel?: () => void };
type Experience = {
  register: (id: string, source: MutableRefObject<Draft>) => () => void;
  requestLeave: (action: () => void, ids?: string[], cancel?: () => void) => void;
  resetDrafts: (ids?: string[]) => void;
  announce: (kind: "success" | "error" | "loading" | "info", message?: string) => void;
};
const fallback: Experience = { register: () => () => {}, requestLeave: action => action(), resetDrafts: () => {}, announce: () => {} };
const Context = createContext<Experience>(fallback);
export const useSettingsExperience = () => useContext(Context);
export function useSettingsNotice() {
  const experience = useSettingsExperience();
  const toast = useToast();
  const announce = experience.announce;
  const nativeToast = toast.showToast;
  const connected = experience !== fallback;
  const showToast = useCallback<ToastContextValue["showToast"]>((message, level = "info", options) => {
    if (!connected) nativeToast(message, level, options);
    else announce(level === "error" ? "error" : level === "warn" ? "info" : "success", message);
  }, [announce, connected, nativeToast]);
  return { showToast };
}

/** Register only persisted drafts, never search text, filters, or previews. */
export function useSettingsDraft(draft: Draft) {
  const experience = useSettingsExperience();
  const id = useId();
  const latest = useRef(draft);
  latest.current = draft;
  useLayoutEffect(() => experience.register(id, latest), [experience.register, id, draft.dirty, draft.busy, draft.canSave]);
  return {
    leave: (action: () => void) => experience.requestLeave(action, [id]),
    discard: () => experience.resetDrafts([id]),
  };
}

export function SettingsExperienceProvider({ children }: { children: ReactNode }) {
  const t = useT();
  const registry = useRef(new Map<string, MutableRefObject<Draft>>());
  const pendingRef = useRef<LeaveRequest | null>(null);
  const [pending, setPending] = useState<LeaveRequest | null>(null);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [failure, setFailure] = useState(false);
  const [, refresh] = useState(0);
  const [notice, setNotice] = useState<{kind: "success" | "error" | "loading" | "info"; message?: string} | null>(null);
  const register = useCallback<Experience["register"]>((id, source) => {
    registry.current.set(id, source);
    refresh(n => n + 1);
    return () => { registry.current.delete(id); };
  }, []);
  const drafts = useCallback((ids?: string[]) => [...registry.current.entries()].filter(([id, ref]) => (!ids || ids.includes(id)) && ref.current.dirty), []);
  const requestLeave = useCallback<Experience["requestLeave"]>((action, ids, cancel) => {
    if (pendingRef.current) return;
    setNotice(null);
    if (!drafts(ids).length) { action(); return; }
    const request = { action, ids, cancel };
    pendingRef.current = request; setFailure(false); setPending(request);
  }, [drafts]);
  const resetDrafts = useCallback((ids?: string[]) => {
    for (const [, ref] of drafts(ids)) { ref.current.discard?.(); ref.current.dirty = false; }
  }, [drafts]);
  const announce = useCallback<Experience["announce"]>((kind, message) => setNotice({ kind, message }), []);
  useEffect(() => {
    const protect = (event: BeforeUnloadEvent) => {
      if (!drafts().length) return;
      event.preventDefault(); event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [drafts]);
  useEffect(() => {
    if (!notice || notice.kind !== "success") return;
    const timer = window.setTimeout(() => setNotice(current => current === notice ? null : current), 4000);
    return () => window.clearTimeout(timer);
  }, [notice]);
  const stay = () => {
    if (savingRef.current) return;
    pendingRef.current?.cancel?.(); pendingRef.current = null; setPending(null);
  };
  const finish = async (save: boolean) => {
    const request = pendingRef.current;
    if (!request || savingRef.current) return;
    const entries = drafts(request.ids);
    if (entries.some(([, ref]) => ref.current.busy)) return;
    savingRef.current = true; setSaving(true); setFailure(false);
    try {
      if (save) {
        // Re-read after every save: a clean parent can become dirty when its
        // child applies. Registration/effect order is not component depth.
        while (drafts(request.ids).length) {
          const [id] = drafts(request.ids).sort((a, b) => (b[1].current.order ?? 0) - (a[1].current.order ?? 0))[0];
          const source = registry.current.get(id);
          if (!source?.current.dirty) continue;
          if (!source.current.save || source.current.canSave === false || !await source.current.save()) { setFailure(true); return; }
          flushSync(() => refresh(n => n + 1));
          if (registry.current.get(id)?.current.dirty) {
            // A newer edit arrived while the write was in flight.
            setFailure(true); return;
          }
        }
      } else { flushSync(() => resetDrafts(request.ids)); }
      pendingRef.current = null; setPending(null);
      request.action();
    } catch { setFailure(true); }
    finally { savingRef.current = false; setSaving(false); }
  };
  const entries = pending ? drafts(pending.ids) : [];
  const blocked = saving || entries.some(([, ref]) => ref.current.busy);
  const canSave = entries.length > 0 && entries.every(([, ref]) => ref.current.save && ref.current.canSave !== false);
  return <Context.Provider value={{ register, requestLeave, resetDrafts, announce }}>
    {children}
    {notice && <div className="settings-operation-feedback"><SettingsFeedback kind={notice.kind}>{notice.message ?? t(notice.kind === "loading" ? "settings.savingChanges" : notice.kind === "error" ? "settings.saveFailedKeepDraft" : "settings.changesSaved")}</SettingsFeedback></div>}
    {pending && <LeaveDialog onCancel={stay}>
      <h2>{t("settings.unsavedTitle")}</h2>
      <p>{t("settings.unsavedDescription")}</p>
      <ul>{[...new Set(entries.map(([, ref]) => ref.current.label))].map(label => <li key={label}>{label}</li>)}</ul>
      {failure && <SettingsFeedback kind="error">{t("settings.saveFailedKeepDraft")}</SettingsFeedback>}
      <div className="settings-save-bar">
        <button type="button" className="btn" autoFocus disabled={saving} onClick={stay}>{t("settings.keepEditing")}</button>
        <button type="button" className="btn btn--danger" disabled={blocked} onClick={() => void finish(false)}>{t("settings.discardAndLeave")}</button>
        <button type="button" className="btn btn--primary" disabled={blocked || !canSave} onClick={() => void finish(true)}>{t(saving ? "settings.savingChanges" : "settings.saveAndLeave")}</button>
      </div>
    </LeaveDialog>}
  </Context.Provider>;
}

function LeaveDialog({ children, onCancel }: { children: ReactNode; onCancel: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const t = useT();
  useLayoutEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    if (ref.current?.showModal) ref.current.showModal(); else ref.current?.setAttribute("open", "");
    ref.current?.querySelector<HTMLButtonElement>("[autofocus]")?.focus();
    return () => { if (previous?.isConnected) previous.focus(); };
  }, []);
  return createPortal(<dialog ref={ref} className="settings-leave-dialog" data-app-overlay="" aria-label={t("settings.unsavedTitle")} onCancel={event => { event.preventDefault(); onCancel(); }}>
    <div className="settings-leave-dialog__content">{children}</div>
  </dialog>, document.body);
}
