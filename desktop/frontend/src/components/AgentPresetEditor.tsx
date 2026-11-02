import { useEffect, useId, useRef, useState } from "react";
import { AGENT_PRESET_ID_PATTERN, suggestedAgentPresetID, type AgentPreset, type WritableAgentPresetScope } from "../lib/agentPresets";
import { useT } from "../lib/i18n";
import { AgentPresetIconPicker } from "./AgentPresetIconPicker";
import { AgentPresetToolPicker } from "./AgentPresetToolPicker";
import { SettingsField, SettingsSection } from "./SettingsForm";
import { SettingsSelect } from "./SettingsSelect";
import { SettingsSubpageHeader, useSettingsDraft } from "./SettingsExperience";

export type AgentPresetDraft = Pick<AgentPreset, "id" | "name" | "description" | "icon" | "iconImage" | "iconColor" | "body" | "keepCoding" | "complete" | "includeRuntimeContext" | "tools"> & { scope: WritableAgentPresetScope };

export function AgentPresetEditor({ initial, editing, presets, busy, onSave, onCancel }: {
  initial: AgentPresetDraft;
  editing: boolean;
  presets: AgentPreset[];
  busy: boolean;
  onSave: (draft: AgentPresetDraft) => Promise<boolean | void>;
  onCancel: () => void;
}) {
  const t = useT();
  const prefix = useId();
  const nameRef = useRef<HTMLInputElement>(null);
  const [draft, setDraft] = useState(initial);
  const [idEdited, setIdEdited] = useState(editing || !!initial.id);
  const [imageEditing, setImageEditing] = useState(false);
  // Unchecking "restrict" keeps the list so re-checking restores it.
  const [lastTools, setLastTools] = useState(initial.tools ?? []);
  const displayedID = idEdited ? draft.id : suggestedAgentPresetID(draft.name, draft.scope, presets);
  const id = displayedID.trim();
  const idInvalid = !AGENT_PRESET_ID_PATTERN.test(id) || id === "default";
  const idTaken = !editing && presets.some(preset => preset.id === id && preset.scope === draft.scope);
  const bodyRequired = draft.complete || !draft.keepCoding;
  const promptMode = draft.complete ? "complete" : draft.keepCoding ? "append" : "replace";
  const replacementMissing = bodyRequired && !draft.body.trim();
  const invalid = idInvalid || idTaken || !draft.name.trim() || replacementMissing || imageEditing;
  const patch = (next: Partial<AgentPresetDraft>) => setDraft(previous => ({ ...previous, ...next }));
  const dirty = imageEditing || JSON.stringify(draft) !== JSON.stringify(initial);
  const guard = useSettingsDraft({ label: t(editing ? "agentPresets.edit" : "agentPresets.new"), dirty, busy, canSave: !invalid,
    save: async () => !invalid && await onSave({ ...draft, id }) !== false, discard: () => setDraft(initial) });
  useEffect(() => { nameRef.current?.focus(); }, []);

  return <form className="agent-preset-editor" aria-label={t(editing ? "agentPresets.edit" : "agentPresets.new")}
    onSubmit={event => { event.preventDefault(); if (!invalid && !busy && (!editing || dirty)) void onSave({ ...draft, id }); }}>
    <SettingsSubpageHeader title={t(editing ? "agentPresets.edit" : "agentPresets.new")} description={t("agentPresets.editHint")} backLabel={t("agentPresets.back")} onBack={onCancel} />
    <SettingsSection>
      <SettingsField label={<label htmlFor={`${prefix}-name`}>{t("agentPresets.name")}</label>}>
        <input ref={nameRef} id={`${prefix}-name`} className="mem-input set-grow" required value={draft.name} disabled={busy} onChange={event => patch({ name: event.target.value })} />
      </SettingsField>
      <SettingsField label={<label htmlFor={`${prefix}-id`}>{t("agentPresets.id")}</label>} hint={t(editing ? "agentPresets.idFixed" : "agentPresets.idHint")}>
        <div className="agent-preset-editor__field">
          <input id={`${prefix}-id`} className="mem-input set-grow" required value={displayedID} disabled={editing || busy} autoCapitalize="none" spellCheck={false}
            aria-invalid={!!id && (idInvalid || idTaken)} aria-describedby={`${prefix}-id-error`}
            onChange={event => { setIdEdited(true); patch({ id: event.target.value }); }} />
          <span id={`${prefix}-id-error`} className="agent-preset-editor__error" role="status">{id && idInvalid ? t("agentPresets.idInvalid") : idTaken ? t("agentPresets.idTaken") : ""}</span>
        </div>
      </SettingsField>
      <SettingsField label={<label htmlFor={`${prefix}-description`}>{t("agentPresets.description")}</label>}>
        <input id={`${prefix}-description`} className="mem-input set-grow" value={draft.description} disabled={busy} onChange={event => patch({ description: event.target.value })} />
      </SettingsField>
      <SettingsField label={t("agentPresets.icon")}>
        <AgentPresetIconPicker icon={draft.icon} image={draft.iconImage} color={draft.iconColor} disabled={busy}
          onChange={(icon, iconImage, iconColor) => patch({ icon, iconImage, iconColor })} onEditingChange={setImageEditing} />
      </SettingsField>
      <SettingsField label={t("agentPresets.scope")} hint={t("agentPresets.scopeHint")}>
        <SettingsSelect value={draft.scope} disabled={editing || busy} aria-label={t("agentPresets.scope")}
          onValueChange={value => patch({ scope: value as WritableAgentPresetScope })}
          options={[{ value: "user", label: t("agentPresets.scope.user") }, { value: "project", label: t("agentPresets.scope.project") }]} />
      </SettingsField>
      <SettingsField label={t("agentPresets.promptMode")} stacked>
        <div className="agent-preset-modes" role="radiogroup" aria-label={t("agentPresets.promptMode")}>
          {(["append", "replace", "complete"] as const).map(mode => <label key={mode} className="agent-preset-modes__option">
            <input type="radio" name={`${prefix}-mode`} value={mode} checked={promptMode === mode} disabled={busy}
              aria-label={t(`agentPresets.mode.${mode}`)}
              aria-describedby={`${prefix}-${mode}-hint`} onChange={() => patch(mode === "complete" ? { complete: true } : { complete: false, keepCoding: mode === "append" })} />
            <span><strong>{t(`agentPresets.mode.${mode}`)}</strong><span id={`${prefix}-${mode}-hint`}>{t(`agentPresets.modeHint.${mode}`)}</span></span>
          </label>)}
        </div>
      </SettingsField>
      <SettingsField label={<label htmlFor={`${prefix}-body`}>{t(`agentPresets.bodyLabel.${promptMode}`)}</label>} stacked>
        <textarea id={`${prefix}-body`} className="mem-textarea" rows={8} value={draft.body} disabled={busy} required={bodyRequired}
          aria-invalid={replacementMissing} aria-describedby={`${prefix}-body-hint`} onChange={event => patch({ body: event.target.value })} />
        <span id={`${prefix}-body-hint`} className={replacementMissing ? "agent-preset-editor__error" : "set-hint"}>{t(replacementMissing ? draft.complete ? "agentPresets.completeBodyRequired" : "agentPresets.bodyRequired" : "agentPresets.markdownHint")}</span>
      </SettingsField>
      <SettingsField label={<label htmlFor={`${prefix}-restrict`}>{t("agentPresets.restrictTools")}</label>} hint={t("agentPresets.toolsHint")}>
        <input id={`${prefix}-restrict`} className="provider-capability-row__switch" type="checkbox" role="switch" checked={draft.tools !== null} disabled={busy}
          onChange={event => patch({ tools: event.target.checked ? lastTools : null })} />
      </SettingsField>
      {draft.tools !== null && <SettingsField label={t("agentPresets.tools")} stacked>
          <AgentPresetToolPicker id={`${prefix}-tools`} value={draft.tools} disabled={busy}
            onChange={tools => { setLastTools(tools); patch({ tools }); }} />
          <div className="agent-preset-editor__tools-footer">
            <span id={`${prefix}-tools-hint`} className="set-hint" role="status">{t(draft.tools.length ? "agentPresets.toolListHint" : "agentPresets.noTools")}</span>
            <button type="button" className="btn btn--small" disabled={busy || !draft.tools.length} onClick={() => { setLastTools([]); patch({ tools: [] }); }}>{t("agentPresets.clearTools")}</button>
          </div>
      </SettingsField>}
      <details className="agent-preset-advanced">
        <summary>{t("agentPresets.advanced")}</summary>
        <SettingsField label={<label htmlFor={`${prefix}-runtime-context`}>{t("agentPresets.includeRuntimeContext")}</label>} hint={<span id={`${prefix}-runtime-context-hint`}>{t("agentPresets.includeRuntimeContextHint")}</span>}>
          <input id={`${prefix}-runtime-context`} type="checkbox" role="switch" className="provider-capability-row__switch"
            checked={draft.includeRuntimeContext} disabled={busy} aria-describedby={`${prefix}-runtime-context-hint`}
            onChange={event => patch({ includeRuntimeContext: event.target.checked })} />
        </SettingsField>
      </details>
    </SettingsSection>
    <div className="settings-save-bar">
      <span role="status">{t(dirty ? "settings.models.unsaved" : "settings.models.saved")}</span>
      <button className="btn btn--small" type="button" disabled={busy} onClick={() => guard.leave(onCancel)}>{t("common.cancel")}</button>
      <button className="btn btn--primary btn--small" type="submit" disabled={busy || invalid || (editing && !dirty)}>{t(busy ? "agentPresets.saving" : "common.save")}</button>
    </div>
  </form>;
}
