import { useEffect, useId, useRef, useState } from "react";
import { Upload } from "lucide-react";
import { useT } from "../lib/i18n";
import { decodePresetIcon, drawPresetIcon, isPresetIconImage, PRESET_ICON_SIZE } from "../lib/agentPresetImage";
import { AGENT_PRESET_ICONS, AgentPresetIcon, DEFAULT_AGENT_PRESET_ICON } from "./AgentPresetIcon";

export function AgentPresetIconPicker({ icon, image, disabled, onChange, onEditingChange }: {
  icon?: string; image?: string; disabled: boolean;
  onChange: (icon: string, image: string) => void;
  onEditingChange: (editing: boolean) => void;
}) {
  const t = useT();
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const preview = useRef<HTMLCanvasElement>(null);
  const request = useRef(0);
  const [source, setSource] = useState<ImageBitmap | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [zoom, setZoom] = useState(1);
  const [x, setX] = useState(50);
  const [y, setY] = useState(50);
  useEffect(() => () => { request.current += 1; }, []);
  useEffect(() => () => source?.close(), [source]);
  useEffect(() => {
    if (source && preview.current) drawPresetIcon(preview.current, source, zoom, x, y);
  }, [source, zoom, x, y]);
  const cancel = () => {
    request.current += 1;
    setSource(null); setLoading(false); setError(""); onEditingChange(false);
  };
  const upload = async (file: File) => {
    const sequence = ++request.current;
    setSource(null); setLoading(true); setError(""); onEditingChange(true);
    try {
      const bitmap = await decodePresetIcon(file);
      if (sequence !== request.current) { bitmap.close(); return; }
      setZoom(1); setX(50); setY(50); setSource(bitmap);
    } catch (cause) {
      if (sequence !== request.current) return;
      const code = cause instanceof Error ? cause.message : "decode";
      setError(t(code === "type" ? "agentPresets.iconTypeError" : code === "size" ? "agentPresets.iconSizeError"
        : code === "dimensions" ? "agentPresets.iconDimensionsError" : "agentPresets.iconDecodeError"));
      onEditingChange(false);
    } finally { if (sequence === request.current) setLoading(false); }
  };
  const apply = () => {
    if (!source) return;
    try {
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = PRESET_ICON_SIZE;
      drawPresetIcon(canvas, source, zoom, x, y);
      const value = canvas.toDataURL("image/png");
      if (!isPresetIconImage(value)) throw new Error("invalid image");
      onChange("", value); cancel();
    } catch { setError(t("agentPresets.iconDecodeError")); }
  };
  return <div className="agent-preset-icon-picker" aria-busy={loading}>
    <div className="agent-preset-editor__icons" role="radiogroup" aria-label={t("agentPresets.icon")}>
      {Object.keys(AGENT_PRESET_ICONS).map(key => <button type="button" key={key} role="radio"
        aria-checked={!image && (icon || DEFAULT_AGENT_PRESET_ICON) === key} aria-label={key} title={key} disabled={disabled}
        className={`agent-preset-editor__icon${!image && (icon || DEFAULT_AGENT_PRESET_ICON) === key ? " agent-preset-editor__icon--on" : ""}`}
        onClick={() => { cancel(); onChange(key === DEFAULT_AGENT_PRESET_ICON ? "" : key, ""); }}><AgentPresetIcon icon={key} /></button>)}
      {image && <button type="button" role="radio" aria-checked="true" aria-label={t("agentPresets.customIcon")} disabled={disabled}
        className="agent-preset-editor__icon agent-preset-editor__icon--on"><AgentPresetIcon image={image} size={24} /></button>}
    </div>
    <div className="agent-preset-icon-picker__actions">
      <input ref={input} type="file" accept="image/png,image/jpeg,image/webp" hidden disabled={disabled}
        aria-label={t("agentPresets.uploadIcon")} onChange={event => {
          const file = event.currentTarget.files?.[0]; event.currentTarget.value = "";
          if (file) void upload(file);
        }} />
      <button type="button" className="btn btn--small" disabled={disabled} onClick={() => input.current?.click()}>
        <Upload size={14} aria-hidden="true" />{t(image ? "agentPresets.replaceIcon" : "agentPresets.uploadIcon")}
      </button>
      {!!image && <button type="button" className="btn btn--small" disabled={disabled}
        onClick={() => { cancel(); onChange("", ""); }}>{t("agentPresets.resetIcon")}</button>}
    </div>
    <span className="set-hint">{t("agentPresets.uploadIconHint")}</span>
    {loading && <span role="status">{t("agentPresets.iconLoading")}</span>}
    {error && <span className="agent-preset-editor__error" role="alert">{error}</span>}
    {(source || loading) && <div className="agent-preset-icon-crop" role="group" aria-label={t("agentPresets.cropIcon")}>
      {source && <>
        <canvas ref={preview} width={192} height={192} role="img" aria-label={t("agentPresets.iconPreview")} />
        <div className="agent-preset-icon-crop__controls">
          {([{ key: "zoom", label: t("agentPresets.iconZoom"), value: zoom, set: setZoom, min: 1, max: 3, step: 0.05 },
            { key: "x", label: t("agentPresets.iconHorizontal"), value: x, set: setX, min: 0, max: 100, step: 1 },
            { key: "y", label: t("agentPresets.iconVertical"), value: y, set: setY, min: 0, max: 100, step: 1 }]).map(control =>
            <label key={control.key} htmlFor={`${id}-${control.key}`}>{control.label}
              <input id={`${id}-${control.key}`} type="range" min={control.min} max={control.max} step={control.step}
                value={control.value} disabled={disabled} onChange={event => control.set(Number(event.target.value))} />
            </label>)}
        </div>
        <button type="button" className="btn btn--small" disabled={disabled} onClick={apply}>{t("agentPresets.useIcon")}</button>
      </>}
      <button type="button" className="btn btn--small" disabled={disabled} onClick={cancel}>{t("common.cancel")}</button>
    </div>}
  </div>;
}
