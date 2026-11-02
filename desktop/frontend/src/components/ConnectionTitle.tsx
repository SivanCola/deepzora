import { useRef, useState } from "react";
import { Check, Pencil, X } from "lucide-react";
import { useT } from "../lib/i18n";
import { useSettingsDraft } from "./SettingsExperience";

export function ConnectionTitle({ label, busy, onSave }: {
  label: string; busy: boolean; onSave: (label: string) => Promise<boolean>;
}) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(label);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(false);
  const pending = useRef(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const close = () => { setEditing(false); setError(false); requestAnimationFrame(() => trigger.current?.focus()); };
  const save = async () => {
    if (pending.current || busy) return false;
    pending.current = true; setSaving(true); setError(false);
    try {
      if (await onSave(draft.trim())) { close(); return true; } else { setError(true); return false; }
    } catch { setError(true); return false; }
    finally { pending.current = false; setSaving(false); }
  };
  const guard = useSettingsDraft({ label: t("settings.connections.name"), dirty: editing && draft.trim() !== label, busy: saving || busy,
    canSave: Boolean(draft.trim()), save, discard: close });
  if (!editing) return <button ref={trigger} type="button" className="connection-title" disabled={busy}
    aria-label={`${t("settings.connections.rename")}: ${label}`}
    onClick={() => { setDraft(label); setError(false); setEditing(true); }}>
    {label}<Pencil size={15} aria-hidden="true" />
  </button>;
  return <span className="connection-title-editor">
    <span className="connection-title-editor__row">
      <input className="mem-input" aria-label={t("settings.connections.name")} value={draft}
        ref={input => { if (input) input.focus(); }} disabled={saving || busy}
        onChange={e => setDraft(e.target.value)}
        onKeyDown={e => {
          if (e.nativeEvent.isComposing || e.keyCode === 229) return;
          if (e.key === "Enter") { e.preventDefault(); void save(); }
          if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); if (!saving) guard.leave(close); }
        }} />
      <button type="button" className="btn provider-icon-action" title={t("common.save")} aria-label={t("common.save")} disabled={saving || busy} onClick={() => void save()}><Check size={16} /></button>
      <button type="button" className="btn provider-icon-action" title={t("common.cancel")} aria-label={t("common.cancel")} disabled={saving} onClick={() => guard.leave(close)}><X size={16} /></button>
    </span>
    {error && <small role="alert">{t("settings.connections.renameFailed")}</small>}
  </span>;
}
