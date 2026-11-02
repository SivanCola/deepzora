import { useCallback, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { useSettingsDraft, useSettingsExperience } from "./SettingsExperience";

/** Own instruction-file drafts, persistence and navigation protection together. */
export function useMemoryDocumentEditor({ tabId, busy, setBusy, setError, reload }: {
  tabId: string | null; busy: boolean;
  setBusy: Dispatch<SetStateAction<boolean>>;
  setError: Dispatch<SetStateAction<string | null>>;
  reload: () => Promise<void>;
}) {
  const t = useT();
  const { requestLeave, announce } = useSettingsExperience();
  const [editingPath, setEditingPath] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const baseline = useRef("");
  const dirty = editingPath !== null && draft !== baseline.current;
  const startEdit = useCallback((path: string, body: string) => {
    requestLeave(() => { baseline.current = body; setEditingPath(path); setDraft(body); });
  }, [requestLeave]);
  const saveEdit = useCallback(async () => {
    if (editingPath === null || busy) return false;
    setBusy(true); setError(null);
    try {
      if (tabId) await app.SaveDocForTab(tabId, editingPath, draft);
      else await app.SaveDoc(editingPath, draft);
      await reload();
      setEditingPath(null); announce("success");
      return true;
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error || "Unknown error"));
      return false;
    } finally { setBusy(false); }
  }, [editingPath, busy, draft, reload, tabId, announce, setBusy, setError]);
  useSettingsDraft({ label: t("memory.instructionFiles"), dirty, busy,
    save: saveEdit, discard: () => setEditingPath(null) });
  return { editingPath, draft, setDraft, dirty, startEdit, saveEdit,
    cancelEdit: () => requestLeave(() => setEditingPath(null)) };
}
