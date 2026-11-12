import { useEffect, useState } from "react";
import { app, openExternal } from "../lib/bridge";
import { useI18n } from "../lib/i18n";
import type { PluginView } from "../lib/types";
import { SettingsFeedback } from "./SettingsExperience";

type Entry = {
  id: string;
  spec: string;
  name: string;
  version: string;
  commit: string;
  repo: string;
  path?: string;
  summary: Record<string, string>;
  channel: string;
  upstream?: { spec: string; version: string };
  capabilities?: string[];
};
function sameSource(p: PluginView, e: Entry) {
  const match =
    /^(?:https?:\/\/|git:)github\.com\/([^/]+\/[^/]+?)(?:\.git)?(?:\/tree\/[^/]+(?:\/(.+))?)?\/?$/i.exec(
      p.source || "",
    );
  return (
    !!match &&
    match[1].toLowerCase() === e.repo.toLowerCase() &&
    (match[2] || "") === (e.path || "")
  );
}
export function PluginMarket({
  installed,
  choose,
}: {
  installed: PluginView[];
  choose: (spec: string, p?: PluginView) => void;
}) {
  const { t, locale } = useI18n();
  const [topics, setTopics] = useState(false);
  const [q, setQ] = useState("");
  const [retry, setRetry] = useState(0);
  const [data, setData] = useState<{
    entries: Entry[];
    warning?: string;
  } | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    setData(null);
    setError("");
    app
      .SearchPluginCatalog("", topics)
      .then((raw) => {
        if (live) setData(JSON.parse(raw));
      })
      .catch((e) => {
        if (live) setError(String(e));
      });
    return () => {
      live = false;
    };
  }, [topics, retry]);
  const rows = (data?.entries || []).filter((e) =>
    [
      e.id,
      e.name,
      e.repo,
      ...Object.values(e.summary),
      ...(e.capabilities || []),
    ]
      .join(" ")
      .toLowerCase()
      .includes(q.trim().toLowerCase()),
  );
  return (
    <section aria-label={t("market.discover")}>
      <div className="settings-toolbar">
        <input
          className="mem-input"
          aria-label={t("market.search")}
          placeholder={t("market.search")}
          value={q}
          onChange={(ev) => setQ(ev.target.value)}
        />
        <label>
          <input
            type="checkbox"
            checked={topics}
            onChange={(ev) => setTopics(ev.target.checked)}
          />
          {t("market.include")}
        </label>
        <button
          className="btn btn--small"
          type="button"
          onClick={() =>
            openExternal("https://github.com/SivanCola/deepzora-plugins#readme")
          }
        >
          {t("market.submit")}
        </button>
      </div>
      {error ? (
        <SettingsFeedback kind="error" onRetry={() => setRetry((i) => i + 1)}>
          {error}
        </SettingsFeedback>
      ) : !data ? (
        <SettingsFeedback kind="loading" />
      ) : null}
      {data?.warning && <p role="status">{data.warning}</p>}
      {data && !rows.length && <p className="mem-empty">{t("market.empty")}</p>}
      {rows.map((e) => {
        const p = installed.find((x) => sameSource(x, e));
        const current = p?.commit === e.commit;
        return (
          <article key={e.spec} className="cap-plugin-installer">
            <div className="settings-toolbar">
              <strong>{e.name}</strong>
              <span className="cap-source-badge">
                {t(
                  e.channel === "community"
                    ? "market.reviewed"
                    : e.channel === "unreviewed"
                      ? "market.unreviewed"
                      : "market.custom",
                )}
              </span>
            </div>
            <p>{e.summary[locale] || e.summary.en}</p>
            <button
              type="button"
              className="btn btn--small"
              onClick={() =>
                openExternal(
                  "https://github.com/" +
                    e.repo +
                    "/tree/" +
                    e.commit +
                    (e.path ? "/" + e.path : ""),
                )
              }
            >
              {t("market.source")}
            </button>
            <div className="drawer__summary">
              {e.repo} · v{e.version} · {e.commit.slice(0, 10)}
              {e.capabilities?.length ? " · " + e.capabilities.join(", ") : ""}
            </div>
            <button
              className="btn btn--small"
              type="button"
              disabled={current}
              onClick={() => choose(e.spec, p)}
            >
              {t(
                current
                  ? "caps.installedPlugins"
                  : p
                    ? "market.reviewUpdate"
                    : "caps.pluginPreview",
              )}
            </button>
            {e.upstream && (
              <button
                type="button"
                className="btn btn--small"
                onClick={() => choose(e.upstream!.spec)}
              >
                {t("market.unreviewed")} · v{e.upstream.version}
              </button>
            )}
          </article>
        );
      })}
    </section>
  );
}
