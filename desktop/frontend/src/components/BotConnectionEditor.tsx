import { useEffect, useRef, useState } from "react";
import { QrCode } from "lucide-react";
import { app } from "../lib/bridge";
import {
  useBotText,
  type BotConfig,
  type BotSession,
  type BotLoginView,
  type BotModel,
  type BotWorkspace,
  type BotMember,
} from "../lib/botChannels";
import {
  SettingsFeedback,
  SettingsSubpageHeader,
  useSettingsDraft,
} from "./SettingsExperience";
import { SettingsField, SettingsSection } from "./SettingsForm";
import { SettingsSelect } from "./SettingsSelect";
import { SettingsOptions } from "./SettingsOptions";

export function BotEditor({
  initial,
  sessions,
  models,
  workspaces,
  members,
  fixedMemberId,
  busy,
  onClose,
  onSave,
}: {
  initial: BotConfig & { hasSecret: boolean; enabled?: boolean };
  sessions: BotSession[];
  models: BotModel[];
  workspaces: BotWorkspace[];
  members: BotMember[];
  fixedMemberId?: string;
  busy: boolean;
  onClose: () => void;
  onSave: (
    input: BotConfig & { secret: string; loginId?: string; connect?: boolean },
  ) => Promise<boolean>;
}) {
  const t = useBotText();
  const [draft, setDraft] = useState({ ...initial, secret: "", loginId: "" });
  const dedicated = draft.mode === "dedicated";
  const memberMode = draft.mode === "member";
  const selectedMember = members.find((m) => m.id === draft.memberId);
  const selectedModel = models.find((m) => m.ref === draft.modelRef);
  const selectedProvider =
    selectedModel?.provider ?? draft.modelRef?.split("/")[0] ?? "";
  const services = [...new Map(models.map((m) => [m.provider, m])).values()];
  const [modelTesting, setModelTesting] = useState(false);
  const [modelTestResult, setModelTestResult] = useState<{
    error: boolean;
    text: string;
  } | null>(null);
  const modelTestGeneration = useRef(0);
  useEffect(
    () => () => {
      modelTestGeneration.current++;
    },
    [],
  );
  const changeModel = (modelRef: string) => {
    modelTestGeneration.current++;
    setModelTesting(false);
    setModelTestResult(null);
    setDraft((old) => ({ ...old, modelRef }));
  };
  const testModel = async () => {
    if (!selectedModel?.configured || modelTesting) return;
    const request = ++modelTestGeneration.current;
    setModelTesting(true);
    setModelTestResult(null);
    try {
      await app.TestBotModel(selectedModel.ref);
      if (request === modelTestGeneration.current)
        setModelTestResult({ error: false, text: t.modelPassed });
    } catch (error) {
      if (request === modelTestGeneration.current)
        setModelTestResult({ error: true, text: String(error) });
    } finally {
      if (request === modelTestGeneration.current) setModelTesting(false);
    }
  };
  const inputRef = useRef<HTMLInputElement>(null);
  const qrRef = useRef<HTMLButtonElement>(null);
  const [login, setLogin] = useState<BotLoginView | null>(null);
  const feishuLike =
    initial.provider === "feishu" || initial.provider === "lark";
  const qrSupported = feishuLike || initial.provider === "weixin";
  const loginLabel = feishuLike ? t.feishuLogin : t.login;
  const [loginBusy, setLoginBusy] = useState(false);
  const [loginError, setLoginError] = useState("");
  const loginGeneration = useRef(0);
  const accessRevision = useRef(0);
  const loginAccessRevision = useRef(0);
  useEffect(
    () => () => {
      loginGeneration.current++;
    },
    [],
  );
  const beginLogin = async () => {
    const generation = ++loginGeneration.current;
    loginAccessRevision.current = accessRevision.current;
    setLoginBusy(true);
    setLoginError("");
    setLogin(null);
    setDraft((old) => ({ ...old, loginId: "" }));
    try {
      const result = await (feishuLike
        ? app.BeginFeishuBotLogin(initial.provider)
        : app.BeginWeixinBotLogin());
      if (generation === loginGeneration.current) setLogin(result);
    } catch (cause) {
      if (generation === loginGeneration.current) setLoginError(String(cause));
    } finally {
      if (generation === loginGeneration.current) setLoginBusy(false);
    }
  };
  useEffect(() => {
    if (!login || !["pending", "scanned"].includes(login.status)) return;
    const generation = loginGeneration.current;
    let cancelled = false;
    const timer = setTimeout(
      () => {
        void (
          feishuLike
            ? app.PollFeishuBotLogin(login.id)
            : app.PollWeixinBotLogin(login.id)
        )
          .then((result) => {
            if (cancelled || generation !== loginGeneration.current) return;
            setLogin((old) =>
              old?.id === result.id ? { ...result, image: old.image } : old,
            );
            if (result.status === "success") {
              const userId = result.userId?.trim();
              const fillAccess =
                !!userId &&
                accessRevision.current === loginAccessRevision.current;
              setDraft((old) => ({
                ...old,
                loginId: result.id,
                secret: "",
                ...(result.appId ? { appId: result.appId } : {}),
                ...(result.provider ? { provider: result.provider } : {}),
                ...(fillAccess
                  ? { chatId: result.chatId || userId!, senderId: userId! }
                  : {}),
              }));
            }
          })
          .catch((cause) => {
            if (!cancelled && generation === loginGeneration.current) {
              setLoginError(String(cause));
              setLogin((old) => (old ? { ...old, status: "error" } : old));
            }
          });
      },
      Math.max(1, login.interval ?? 2.5) * 1000,
    );
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [login, feishuLike]);
  useEffect(() => {
    if (qrSupported) qrRef.current?.focus();
    else inputRef.current?.focus();
  }, [qrSupported]);
  const dirty =
    JSON.stringify(draft) !==
    JSON.stringify({ ...initial, secret: "", loginId: "" });
  const valid =
    !!draft.name.trim() &&
    (memberMode
      ? !!selectedMember
      : dedicated
        ? !!selectedModel?.configured &&
          workspaces.some((w) => w.id === draft.workspaceId)
        : !!draft.sessionId) &&
    !!draft.chatId.trim() &&
    !!draft.senderId.trim() &&
    (initial.hasSecret || !!draft.secret.trim() || !!draft.loginId) &&
    (!["feishu", "lark"].includes(draft.provider) || !!draft.appId.trim());
  const save = async (connect = false) =>
    valid &&
    (await onSave({
      ...draft,
      connect,
      name: draft.name.trim(),
      appId: draft.appId.trim(),
      chatId: draft.chatId.trim(),
      senderId: draft.senderId.trim(),
    }));
  const guard = useSettingsDraft({
    label: t.title,
    dirty,
    busy: busy || modelTesting,
    canSave: valid,
    save: () => save(false),
    discard: () => setDraft({ ...initial, secret: "", loginId: "" }),
  });
  const field = (
    key: "name" | "appId" | "chatId" | "senderId" | "secret",
    label: string,
    hint?: string,
  ) => (
    <SettingsField label={label} hint={hint}>
      <input
        className="mem-input set-grow"
        ref={key === "name" ? inputRef : undefined}
        value={draft[key]}
        disabled={busy}
        type={key === "secret" ? "password" : "text"}
        autoComplete="off"
        spellCheck={false}
        maxLength={key === "secret" ? 8192 : key === "name" ? 120 : 256}
        onChange={(e) => {
          if (key === "chatId" || key === "senderId") accessRevision.current++;
          if (key === "secret" || key === "appId") {
            loginGeneration.current++;
            setLogin(null);
            setLoginBusy(false);
            setLoginError("");
          }
          setDraft((old) => ({
            ...old,
            [key]: e.target.value,
            ...(key === "secret" || key === "appId" ? { loginId: "" } : {}),
          }));
        }}
      />
    </SettingsField>
  );
  return (
    <form
      className="bot-channel-editor"
      onSubmit={(e) => {
        e.preventDefault();
        if (!busy && !modelTesting && (dirty || !initial.enabled) && valid)
          void save(true);
      }}
    >
      <SettingsSubpageHeader
        title={`${t[initial.provider]} · ${initial.hasSecret ? t.edit : t.new}`}
        backLabel={t.back}
        onBack={onClose}
        description={
          t[
            initial.provider === "telegram"
              ? "telegramHelp"
              : initial.provider === "weixin"
                ? "weixinHelp"
                : "feishuHelp"
          ]
        }
      />
      <SettingsSection>
        {qrSupported && (
          <div className="bot-login">
            <button
              ref={qrRef}
              className="btn btn--primary"
              type="button"
              disabled={busy || loginBusy}
              onClick={() => void beginLogin()}
            >
              <QrCode size={18} aria-hidden="true" />
              {loginLabel}
            </button>
            {loginBusy && <SettingsFeedback kind="loading" />}
            {loginError && (
              <SettingsFeedback kind="error">{loginError}</SettingsFeedback>
            )}
            {login && login.status !== "error" && (
              <>
                <SettingsFeedback
                  kind={login.status === "success" ? "success" : "info"}
                >
                  {login.status === "success"
                    ? feishuLike
                      ? login.userId
                        ? t.feishuSuccess
                        : t.feishuMissing
                      : login.userId
                        ? draft.chatId === login.userId &&
                          draft.senderId === login.userId
                          ? t.accessIdentified
                          : t.authorized
                        : t.accessMissing
                    : login.status === "denied"
                      ? t.denied
                      : login.status === "expired"
                        ? t.expired
                        : login.status === "scanned"
                          ? t.scanned
                          : feishuLike
                            ? t.feishuPending
                            : t.pending}
                </SettingsFeedback>
                {["pending", "scanned"].includes(login.status) && (
                  <img
                    width={256}
                    height={256}
                    src={login.image}
                    alt={loginLabel}
                  />
                )}
              </>
            )}
          </div>
        )}
        {field("name", t.name)}
        {memberMode ? (
          <>
            <SettingsField label={t.member} hint={t.memberFixed}>
              <SettingsSelect
                value={draft.memberId || ""}
                disabled={busy || !!fixedMemberId || initial.hasSecret}
                options={members.map((m) => ({ value: m.id, label: m.name }))}
                onValueChange={(memberId) =>
                  setDraft((old) => ({ ...old, memberId }))
                }
              />
            </SettingsField>
            <SettingsFeedback kind={selectedMember ? "info" : "error"}>
              {selectedMember ? t.memberHint : t.memberMissing}
            </SettingsFeedback>
            {selectedMember && (
              <p>
                {t.model}: {selectedMember.model || "—"} ·{" "}
                {selectedMember.permission}
              </p>
            )}
          </>
        ) : (
          <>
            <SettingsField label={t.modelMode}>
              <SettingsOptions layout="fill" aria-label={t.modelMode}>
                <button
                  disabled={busy || modelTesting}
                  aria-pressed={!dedicated}
                  onClick={() =>
                    setDraft((old) => ({
                      ...old,
                      mode: "follow",
                      sessionId: sessions.some((s) => s.id === old.sessionId)
                        ? old.sessionId
                        : sessions[0]?.id || "",
                    }))
                  }
                >
                  {t.follow}
                </button>
                <button
                  disabled={busy || modelTesting}
                  aria-pressed={dedicated}
                  onClick={() =>
                    setDraft((old) => ({
                      ...old,
                      mode: "dedicated",
                      modelRef:
                        old.modelRef ||
                        models.find((m) => m.configured)?.ref ||
                        "",
                      workspaceId:
                        old.workspaceId ||
                        workspaces.find(
                          (w) =>
                            w.root &&
                            w.root ===
                              sessions.find((s) => s.id === old.sessionId)
                                ?.workspace,
                        )?.id ||
                        "global",
                    }))
                  }
                >
                  {t.dedicated}
                </button>
              </SettingsOptions>
            </SettingsField>
            {dedicated ? (
              <>
                <SettingsFeedback kind="info">
                  {t.dedicatedHint}
                </SettingsFeedback>
                <SettingsField label={t.workspace}>
                  <SettingsSelect
                    value={draft.workspaceId || ""}
                    disabled={busy || modelTesting}
                    options={workspaces.map((w) => ({
                      value: w.id,
                      label:
                        w.id === "global"
                          ? t.globalWorkspace
                          : w.title || w.root,
                      hint: w.root,
                    }))}
                    onValueChange={(workspaceId) =>
                      setDraft((old) => ({ ...old, workspaceId }))
                    }
                  />
                </SettingsField>
                <SettingsField label={t.modelService}>
                  <SettingsSelect
                    value={selectedProvider}
                    disabled={busy || modelTesting}
                    options={services.map((m) => ({
                      value: m.provider,
                      label: m.providerLabel,
                    }))}
                    onValueChange={(provider) =>
                      changeModel(
                        models.find((m) => m.provider === provider)?.ref || "",
                      )
                    }
                  />
                </SettingsField>
                <SettingsField label={t.model} hint={t.modelHint}>
                  <SettingsSelect
                    value={draft.modelRef || ""}
                    disabled={busy || modelTesting}
                    selectedLabel={
                      !selectedModel && draft.modelRef
                        ? t.modelUnavailable
                        : undefined
                    }
                    options={models
                      .filter((m) => m.provider === selectedProvider)
                      .map((m) => ({
                        value: m.ref,
                        label: m.model,
                        hint: m.configured ? undefined : t.missingKey,
                      }))}
                    onValueChange={changeModel}
                  />
                </SettingsField>
                {!models.length && (
                  <SettingsFeedback kind="info">{t.noModels}</SettingsFeedback>
                )}
                {selectedModel && !selectedModel.configured && (
                  <SettingsFeedback kind="error">
                    {t.missingKey}
                  </SettingsFeedback>
                )}
                <button
                  className="btn"
                  type="button"
                  disabled={busy || modelTesting || !selectedModel?.configured}
                  onClick={() => void testModel()}
                >
                  {modelTesting ? t.testingModel : t.testModel}
                </button>
                {modelTestResult && (
                  <SettingsFeedback
                    kind={modelTestResult.error ? "error" : "success"}
                  >
                    {modelTestResult.text}
                  </SettingsFeedback>
                )}
              </>
            ) : (
              <>
                <SettingsField label={t.session} hint={t.sessionHint}>
                  <SettingsSelect
                    value={draft.sessionId}
                    disabled={busy}
                    options={[
                      ...sessions.map((s) => ({
                        value: s.id,
                        label: s.title || s.id,
                        hint: s.workspace,
                      })),
                      ...(!sessions.some((s) => s.id === draft.sessionId) &&
                      draft.sessionId
                        ? [
                            {
                              value: draft.sessionId,
                              label: draft.sessionId,
                              hint: t.noSession,
                            },
                          ]
                        : []),
                    ]}
                    onValueChange={(sessionId) =>
                      setDraft((old) => ({ ...old, sessionId }))
                    }
                  />
                </SettingsField>
                {!sessions.length && (
                  <SettingsFeedback kind="info">{t.noSession}</SettingsFeedback>
                )}
              </>
            )}
          </>
        )}
        {["feishu", "lark"].includes(initial.provider) &&
          field("appId", t.appId)}
        {field("secret", t.secret, t.secretHint)}
        {field(
          "chatId",
          t.chat,
          initial.provider === "weixin"
            ? t.weixinAccessHint
            : feishuLike
              ? t.feishuAccessHint
              : t.accessHint,
        )}
        {field("senderId", t.sender)}
      </SettingsSection>
      <div className="settings-save-bar">
        <span role="status">{dirty ? t.dirty : t.clean}</span>
        <div className="settings-save-bar__actions">
          <button
            type="button"
            className="btn"
            disabled={busy}
            onClick={() => guard.leave(onClose)}
          >
            {t.cancel}
          </button>
          {!initial.enabled && (
            <button
              type="button"
              className="btn"
              disabled={busy || modelTesting || !dirty || !valid}
              onClick={() => void save(false)}
            >
              {t.saveOnly}
            </button>
          )}
          <button
            type="submit"
            className="btn btn--primary"
            disabled={
              busy || modelTesting || (!dirty && !!initial.enabled) || !valid
            }
          >
            {initial.enabled ? t.saveReconnect : t.saveConnect}
          </button>
        </div>
      </div>
    </form>
  );
}
