import { useCallback, useEffect, useRef, useState } from "react";
import { Bot, Smartphone, RefreshCw, ArrowUpRight } from "lucide-react";
import { app } from "../lib/bridge";
import {
  botProviders,
  useBotText,
  type BotChannelsView,
  type BotConfig,
  type BotConnection,
  type BotProvider,
  type BotModel,
} from "../lib/botChannels";
import {
  SettingsFeedback,
} from "./SettingsExperience";
import { SettingsSection } from "./SettingsForm";
import { InlineConfirmButton } from "./InlineConfirmButton";
import { BotEditor } from "./BotConnectionEditor";
import "./BotChannelsSettingsPage.css";

function ChannelIcon({ provider }: { provider: BotProvider }) {
  return (
    <img
      className="bot-channel-icon"
      src={`/bot-channel-icons/${provider === "lark" ? "feishu" : provider}.png`}
      alt=""
    />
  );
}
export function BotChannelsSettingsPage({
  memberId,
}: { memberId?: string } = {}) {
  const t = useBotText();
  const [view, setView] = useState<BotChannelsView | null>(null);
  const [error, setError] = useState("");
  const [loadError, setLoadError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [editor, setEditor] = useState<
    (BotConfig & { hasSecret: boolean; enabled?: boolean }) | null
  >(null);
  const alive = useRef(true);
  const generation = useRef(0);
  const mutation = useRef(false);
  const listRef = useRef<HTMLDivElement>(null);
  const reload = useCallback(async () => {
    const request = ++generation.current;
    try {
      const next = await app.ListBotChannels();
      if (alive.current && request === generation.current) {
        setView(next);
        setLoadError("");
      }
    } catch (cause) {
      if (alive.current && request === generation.current)
        setLoadError(String(cause));
    } finally {
      if (alive.current && request === generation.current) setLoading(false);
    }
  }, []);
  useEffect(() => {
    alive.current = true;
    void reload();
    const timer = setInterval(() => {
      if (!mutation.current) void reload();
    }, 3000);
    return () => {
      alive.current = false;
      generation.current++;
      clearInterval(timer);
    };
  }, [reload]);
  const mutate = async (operation: () => Promise<void>, success = "") => {
    if (mutation.current) return false;
    mutation.current = true;
    generation.current++;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await operation();
      if (!alive.current) return false;
      await reload();
      setNotice(success);
      return true;
    } catch (cause) {
      if (alive.current) setError(String(cause));
      return false;
    } finally {
      mutation.current = false;
      if (alive.current) setBusy(false);
    }
  };
  const add = (provider: BotProvider) =>
    setEditor({
      id: crypto.randomUUID(),
      provider,
      name: t[provider],
      appId: "",
      sessionId: view?.sessions[0]?.id ?? "",
      chatId: "",
      senderId: "",
      hasSecret: false,
      mode: "member",
      memberId: memberId || view?.members?.[0]?.id || "",
    });
  const close = () => {
    setEditor(null);
    requestAnimationFrame(() => listRef.current?.focus());
  };
  return (
    <div className={`bot-channels${memberId ? " bot-channels--member" : ""}`} aria-busy={busy || loading}>
      {loadError && (
        <SettingsFeedback kind="error" onRetry={() => void reload()}>
          {loadError}
        </SettingsFeedback>
      )}
      {error && (
        <SettingsFeedback kind="error" onRetry={() => void reload()}>
          {error}
        </SettingsFeedback>
      )}
      {notice && <SettingsFeedback kind="success">{notice}</SettingsFeedback>}
      {loading && !view ? (
        <SettingsFeedback kind="loading">{t.loading}</SettingsFeedback>
      ) : editor ? (
        <BotEditor
          key={editor.id}
          initial={editor}
          sessions={view?.sessions ?? []}
          models={view?.models ?? []}
          workspaces={view?.workspaces ?? []}
          members={view?.members ?? []}
          fixedMemberId={memberId}
          busy={busy}
          onClose={close}
          onSave={async (input) => {
            const ok = await mutate(
              () => app.SaveBotConnection(input),
              t.saved,
            );
            if (ok) close();
            return ok;
          }}
        />
      ) : (
        <>
          <div className="bot-access-grid">
            {!memberId && (
              <>
                <section className="bot-access-card bot-access-card--mobile">
                  <Smartphone size={26} aria-hidden="true" />
                  <h2>{t.mobile}</h2>
                  <p>{t.later}</p>
                  <div className="bot-access-placeholder" aria-hidden="true">
                    <Smartphone size={64} strokeWidth={1} />
                  </div>
                  <button
                    className="btn"
                    type="button"
                    onClick={() => listRef.current?.focus()}
                  >
                    <Bot size={16} />
                    {t.manage}
                  </button>
                </section>
              </>
            )}
            <section className="bot-access-card">
              <h2>{memberId ? t.channelsSection : t.channels}</h2>
              <p>{t.memberHint}</p>
              {!view?.members?.length && (
                <SettingsFeedback kind="info">{t.noMembers}</SettingsFeedback>
              )}
              <div className="bot-provider-list">
                {botProviders.map((provider) => (
                  <button
                    type="button"
                    className="bot-provider-card"
                    key={provider}
                    disabled={busy || !view || !view.members?.length}
                    onClick={() => add(provider)}
                  >
                    <ChannelIcon provider={provider} />
                    <span>
                      <strong>
                        {t[provider]}{" "}
                        {provider === "feishu" || provider === "lark" ? (
                          <small>
                            {provider === "feishu" ? t.china : t.global}
                          </small>
                        ) : null}
                      </strong>
                      <span>
                        {provider === "telegram"
                          ? t.configure
                          : t.scanConfigure}
                      </span>
                    </span>
                    <ArrowUpRight size={16} aria-hidden="true" />
                  </button>
                ))}
              </div>
            </section>
          </div>
          <div ref={listRef} tabIndex={-1}>
            <SettingsSection
              title={t.manage}
              description={<p>{t.restart}</p>}
              actions={
                <button
                  type="button"
                  className="btn btn--small"
                  aria-label={t.refresh}
                  disabled={busy}
                  onClick={() => void reload()}
                >
                  <RefreshCw size={15} />
                </button>
              }
            >
              {view && view.connections.length === 0 && (
                <SettingsFeedback kind="empty">{t.empty}</SettingsFeedback>
              )}
              {view?.connections
                .filter((c) => !memberId || c.memberId === memberId)
                .map((connection) => (
                  <ConnectionRow
                    key={connection.id}
                    connection={connection}
                    models={view?.models ?? []}
                    onRetry={() =>
                      mutate(() => app.ConnectBotChannel(connection.id))
                    }
                    busy={busy}
                    onEdit={() => {
                      setNotice("");
                      setError("");
                      setEditor(connection);
                    }}
                    onDelete={() =>
                      mutate(() => app.DeleteBotConnection(connection.id))
                    }
                    onToggle={() =>
                      mutate(() =>
                        connection.enabled ||
                        ["connected", "connecting", "reconnecting"].includes(
                          connection.status,
                        )
                          ? app.DisconnectBotChannel(connection.id)
                          : app.ConnectBotChannel(connection.id),
                      )
                    }
                  />
                ))}
            </SettingsSection>
          </div>
          <p className="bot-channel-help">{t.commands}</p>
        </>
      )}
    </div>
  );
}
function ConnectionRow({
  connection: c,
  models,
  busy,
  onEdit,
  onDelete,
  onToggle,
  onRetry,
}: {
  connection: BotConnection;
  models: BotModel[];
  busy: boolean;
  onEdit: () => void;
  onDelete: () => Promise<boolean>;
  onToggle: () => Promise<boolean>;
  onRetry: () => Promise<boolean>;
}) {
  const t = useBotText();
  const connected = ["connected", "connecting", "reconnecting"].includes(
    c.status,
  );
  const status = Object.prototype.hasOwnProperty.call(t, c.status)
    ? t[c.status as keyof typeof t]
    : t.unknown;
  const modelLabel = (ref: string) => {
    const model = models.find((m) => m.ref === ref);
    return model ? `${model.providerLabel} · ${model.model}` : ref;
  };
  return (
    <article className="bot-connection">
      <ChannelIcon provider={c.provider} />
      <div className="bot-connection-copy">
        <h3>{c.name}</h3>
        <p>
          {t[c.provider]} · {status}
        </p>
        <small>{c.chatId}</small>
        <p>
          {c.memberId
            ? `${t.member}: ${c.memberName || c.memberId}`
            : c.mode === "dedicated"
              ? t.dedicated
              : t.follow}
        </p>
        {c.legacySessionId && (
          <button
            className="btn btn--small"
            type="button"
            onClick={() => void app.OpenBotMemberHistory(c.id)}
          >
            {t.originalHistory}
          </button>
        )}
        {c.enabled && <small>{t.enabledHint}</small>}
        {!!((c.received || 0) + (c.sent || 0)) && (
          <small
            title={
              c.lastSentAt ? new Date(c.lastSentAt).toLocaleString() : undefined
            }
          >
            {t.delivery}: {t.received} {c.received || 0} · {t.sent}{" "}
            {c.sent || 0}
          </small>
        )}
        {c.mode === "dedicated" && c.modelRef && (
          <small>
            {t.configuredModel}: {modelLabel(c.modelRef)}
          </small>
        )}
        {c.actualModel && (
          <small>
            {t.actualModel}: {modelLabel(c.actualModel)}
          </small>
        )}
        {c.modelError && (
          <SettingsFeedback kind="error">{c.modelError}</SettingsFeedback>
        )}
        {c.error && <SettingsFeedback kind="error">{c.error}</SettingsFeedback>}
      </div>
      <div className="bot-connection-actions">
        {c.enabled && c.status === "error" && (
          <button
            className="btn btn--small"
            disabled={busy}
            onClick={() => void onRetry()}
          >
            {t.retryConnect}
          </button>
        )}
        <button
          className="btn btn--small"
          disabled={busy}
          onClick={() => void onToggle()}
        >
          {connected || c.enabled ? t.disconnect : t.connect}
        </button>
        <button className="btn btn--small" disabled={busy} onClick={onEdit}>
          {t.edit}
        </button>
        <InlineConfirmButton
          label={t.remove}
          confirmLabel={t.confirm}
          cancelLabel={t.cancel}
          disabled={busy}
          danger
          onConfirm={async () => {
            await onDelete();
          }}
        />
      </div>
    </article>
  );
}
