import { formatTokens } from "../lib/format";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Check, ChevronDown, ChevronRight, Cpu, Image, List, Plus, Search, Star } from "lucide-react";
import { asArray } from "../lib/array";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { readModelFavorites, writeModelFavorites } from "../lib/modelFavorites";
import { providerBrandIcons } from "../lib/providerBrandIcons";
import { providerBrandID } from "../lib/providerBrand";
import type { ModelInfo } from "../lib/types";
import { AnchoredPopover } from "./AnchoredPopover";
import { Tooltip } from "./Tooltip";
import { useModelApplicationStatus } from "../lib/useModelApplicationStatus";

const emptyModels: ModelInfo[] = [];

const magpieSourceBrands = new Map<string, [string, string]>([
  ["ollama-cloud", ["Ollama Cloud", "ollama"]], ["codex", ["Codex", "openai"]],
  ["opencode-go", ["OpenCode Go", "opencode"]], ["zcode", ["ZCode", ""]],
]);

// Only Magpie gives the first segment of a routed ID this meaning. Other
// providers (e.g. OpenRouter) also serve IDs containing slashes.
function magpieSource(model: ModelInfo) {
  if (providerBrandID(model.provider, model.displayName ?? "", model.brandId) !== "magpie") return null;
  const slash = model.model.indexOf("/");
  if (slash < 1 || slash === model.model.length - 1) return null;
  const id = model.model.slice(0, slash);
  const fallback = magpieSourceBrands.get(id);
  const label = model.modelDisplayName?.split(" · ").slice(1).join(" · ").trim() || fallback?.[0] || id;
  return { id: `source:${id}`, upstream: id, label, brandId: fallback?.[1] || providerBrandID(id, label) };
}

// ModelSwitcher opens an upward popover listing configured providers. Selecting
// one switches the active model while the current conversation continues.
export function ModelSwitcher({
  label,
  tabId,
  ready = true,
  sessionKey,
  onPick,
  onManage,
  detailLabel,
  details,
  composerMenu = false,
  disabled = false,
  dismissSignal,
}: {
  label: string;
  detailLabel?: string;
  details?: ReactNode;
  composerMenu?: boolean;
  disabled?: boolean;
  dismissSignal?: number;
  tabId?: string;
  ready?: boolean;
  sessionKey?: string;
  onPick: (name: string) => boolean | Promise<boolean>;
  onManage?: () => void;
}) {
  const t = useT();
  const application=useModelApplicationStatus(tabId);
  const [open, setOpen] = useState(false);
  const catalogScope = JSON.stringify([tabId ?? "", sessionKey ?? ""]);
  const [catalog, setCatalog] = useState<{ scope: string; models: ModelInfo[] }>({ scope: "", models: emptyModels });
  // A new surface must not display the previous session's current model while
  // its own catalog is loading (including rebinding within the same tab).
  const models = catalog.scope === catalogScope ? catalog.models : emptyModels;
  const [query, setQuery] = useState("");
  const [activeFilter, setActiveFilter] = useState("all");
  const [favorites, setFavorites] = useState<Set<string>>(() => readModelFavorites());
  const [triggerWidth, setTriggerWidth] = useState<number | undefined>(undefined);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const loadSeqRef = useRef(0);
  const currentTabKeyRef = useRef(tabId ?? "");
  const currentCatalogScopeRef = useRef(catalogScope);
  const pendingPickCountByTabRef = useRef(new Map<string, number>());
  const pickSeqByTabRef = useRef(new Map<string, number>());
  currentTabKeyRef.current = tabId ?? "";
  currentCatalogScopeRef.current = catalogScope;

  useEffect(() => {
    setOpen(false);
  }, [disabled, dismissSignal, sessionKey, tabId]);

  // Measure trigger width off the render path to avoid forced layout
  useEffect(() => {
    const el = triggerRef.current;
    if (!el) return;
    const measure = () => setTriggerWidth(el.getBoundingClientRect().width);
    measure();
    const observer = new ResizeObserver(() => measure());
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const loadModelsForTab = useCallback((targetTabId?: string) => {
    const targetKey = targetTabId ?? "";
    const seq = ++loadSeqRef.current;
    return (targetTabId ? app.ModelsForTab(targetTabId) : app.Models())
      .then((next) => {
        if (seq === loadSeqRef.current && currentTabKeyRef.current === targetKey && currentCatalogScopeRef.current === catalogScope) {
          setCatalog({ scope: catalogScope, models: asArray(next).map(normalizeModelInfo) });
        }
      })
      .catch(() => {});
  }, [catalogScope]);

  const loadModels = useCallback(
    () => loadModelsForTab(tabId),
    [loadModelsForTab, tabId],
  );

  useEffect(() => {
    void loadModels();
  }, [loadModels, ready, sessionKey, label]);

  useEffect(() => {
    const refresh = () => void loadModels();
    window.addEventListener("deepzora:model-catalog-changed", refresh);
    return () => window.removeEventListener("deepzora:model-catalog-changed", refresh);
  }, [loadModels]);

  useEffect(() => {
    if (open) {
      setQuery("");
      void loadModels();
      window.requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [loadModels, open]);

  const providers = useMemo(() => {
    const seen = new Set<string>();
    return models.flatMap((model) => {
      if (seen.has(model.provider)) return [];
      seen.add(model.provider);
      return [{
        id: model.provider,
        label: model.displayName?.trim() || providerLabel(model.provider, t),
        brandId: model.brandId,
      }];
    });
  }, [models, t]);

  const sourceByRef = useMemo(() => new Map(models.map(model => [model.ref, magpieSource(model)])), [models]);
  const sources = useMemo(() => {
    const unique = new Map<string, NonNullable<ReturnType<typeof magpieSource>>>();
    for (const source of sourceByRef.values()) if (source && !unique.has(source.id)) unique.set(source.id, source);
    return [...unique.values()];
  }, [sourceByRef]);

  useEffect(() => {
    if (activeFilter !== "all" && activeFilter !== "favorites" && ![...providers, ...sources].some((item) => item.id === activeFilter)) {
      setActiveFilter("all");
    }
  }, [activeFilter, providers, sources]);

  const keyword = query.trim().toLowerCase();
  const filtered = useMemo(() => models.filter((model) => {
    if (activeFilter === "favorites" && !favorites.has(model.ref)) return false;
    const source = sourceByRef.get(model.ref);
    if (activeFilter !== "all" && activeFilter !== "favorites" && model.provider !== activeFilter && source?.id !== activeFilter) return false;
    return !keyword
      || model.model.toLowerCase().includes(keyword)
      || (model.modelDisplayName ?? "").toLowerCase().includes(keyword)
      || model.provider.toLowerCase().includes(keyword)
      || !!source?.label.toLowerCase().includes(keyword)
      || (model.displayName ?? "").toLowerCase().includes(keyword);
  }), [activeFilter, favorites, keyword, models, sourceByRef]);

  // Preserve catalog/configuration order, including when the current model changes.
  const groups = useMemo(() => {
    if (activeFilter === "favorites") {
      return [{ id: "favorites", label: t("modelSwitcher.favorites"), items: filtered }];
    }
    const result = new Map<string, {id: string; label: string; items: ModelInfo[]}>();
    if (activeFilter === "all") result.set("favorites", { id: "favorites", label: t("modelSwitcher.favorites"), items: [] });
    const duplicateLabels = new Set(providers.filter((p, i) => providers.some((other, j) => i !== j && p.label === other.label)).map(p => p.label));
    for (const model of filtered) {
      const source = sourceByRef.get(model.ref);
      const connection = model.displayName?.trim() || providerLabel(model.provider, t);
      const name = duplicateLabels.has(connection) ? `${connection} (${model.provider})` : connection;
      const favorite = activeFilter === "all" && favorites.has(model.ref);
      const id = favorite ? "favorites" : source ? JSON.stringify([model.provider, source.id]) : activeFilter;
      const label = source ? `${source.label} · ${name}` : activeFilter === "all" ? t("modelSwitcher.allModels") : name;
      if (!result.has(id)) result.set(id, { id, label, items: [] });
      result.get(id)!.items.push(model);
    }
    return [...result.values()].filter(group => group.items.length > 0);
  }, [activeFilter, favorites, filtered, providers, sourceByRef, t]);

  const currentModel = useMemo(() => models.find((m) => m.current)
    ?? models.find((m) => m.model === label || m.ref === label), [label, models]);
  const currentProvider = currentModel ? (currentModel.displayName?.trim() || providerLabel(currentModel.provider, t)) : null;
  const fallbackLabel = currentModel && (label === currentModel.ref || !label) ? currentModel.model : label;
  const displayLabel = currentModel?.modelDisplayName?.trim() || fallbackLabel;
  const triggerLabel = [displayLabel, currentModel && displayLabel !== currentModel.model ? currentModel.model : null, currentProvider, detailLabel].filter(Boolean).join(" · ");

  const toggleFavorite = (ref: string) => {
    setFavorites((current) => {
      const next = new Set(current);
      if (next.has(ref)) next.delete(ref);
      else next.add(ref);
      writeModelFavorites(next);
      return next;
    });
  };

  const pick = (model: ModelInfo) => {
    setOpen(false);
    const pendingKey = tabId ?? "";
    const pendingPickCount = pendingPickCountByTabRef.current.get(pendingKey) ?? 0;
    // A catalog refresh can still report the outgoing model as current while
    // an earlier switch is rebuilding. In that window, selecting it again is
    // an intentional last-click-wins rollback rather than a no-op.
    if (model.current && pendingPickCount === 0) return;
    const previousModels = models;
    const pickSeq = (pickSeqByTabRef.current.get(pendingKey) ?? 0) + 1;
    pickSeqByTabRef.current.set(pendingKey, pickSeq);
    // Catalog requests started before this click describe the outgoing model
    // and must not overwrite the optimistic last-click choice.
    loadSeqRef.current += 1;
    setCatalog({ scope: catalogScope, models: models.map((m) => ({ ...m, current: m.ref === model.ref })) });
    pendingPickCountByTabRef.current.set(pendingKey, pendingPickCount + 1);
    const settlePick = (switched: boolean) => {
      const nextCount = Math.max(
        0,
        (pendingPickCountByTabRef.current.get(pendingKey) ?? 0) - 1,
      );
      if (nextCount === 0) pendingPickCountByTabRef.current.delete(pendingKey);
      else pendingPickCountByTabRef.current.set(pendingKey, nextCount);
      // A superseded completion no longer owns the visible selection. Only the
      // latest failed click may roll back and reconcile with the backend.
      if (
        switched ||
        pickSeqByTabRef.current.get(pendingKey) !== pickSeq ||
        currentTabKeyRef.current !== pendingKey || currentCatalogScopeRef.current !== catalogScope
      ) {
        return;
      }
      setCatalog({ scope: catalogScope, models: previousModels });
      void loadModelsForTab(tabId);
    };
    try {
      void Promise.resolve(onPick(model.ref)).then(
        (switched) => settlePick(switched),
        () => settlePick(false),
      );
    } catch (err) {
      settlePick(false);
      throw err;
    }
  };

  return (
    <div className="modelsw">
      <Tooltip label={triggerLabel} fill disabled={open}>
        <button
          ref={triggerRef}
          type="button"
          className="modelsw__trigger"
          disabled={disabled}
          aria-label={triggerLabel}
          aria-expanded={open && !disabled}
          onClick={() => setOpen((v) => !v)}
        >
          <Cpu size={14} className="modelsw__kind" />
          <span className="modelsw__label">{displayLabel}{detailLabel && <span className="modelsw__detail"> · {detailLabel}</span>}{application?.application==="pending" && <span className="modelsw__detail"> · {t("modelApply.badge")}</span>}</span>
          <ChevronDown size={12} />
        </button>
      </Tooltip>
      <AnchoredPopover
        open={open && !disabled}
        anchorRef={triggerRef}
        onClose={() => setOpen(false)}
        className={`modelsw__menu modelsw__menu--portal${composerMenu ? " composer-menu-surface" : ""}`}
        style={composerMenu ? undefined : { minWidth: Math.max(triggerWidth || 200, 200), maxWidth: "min(90vw, 480px)" }}
      >
        <div className="modelsw__search" role="presentation">
            <Search size={17} />
            <input
              ref={inputRef}
              type="text"
              className="modelsw__search-input"
              placeholder={t("modelSwitcher.searchPlaceholder")}
              aria-label={t("modelSwitcher.searchPlaceholder")}
              value={query}
              onChange={(e) => {
                const nextQuery = e.target.value;
                setQuery(nextQuery);
                // The search field belongs to the whole catalog. Typing while
                // a provider or Favorites is selected must still find models
                // from every configured connection.
                if (nextQuery.trim()) setActiveFilter("all");
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape") setOpen(false);
                if (e.key === "Enter" && filtered.length === 1) pick(filtered[0]);
              }}
            />
        </div>
        <div className="modelsw__body">
          <nav className="modelsw__rail" aria-label={t("modelSwitcher.filters")}>
            <button type="button" className="modelsw__rail-item" aria-label={t("modelSwitcher.favorites")} title={t("modelSwitcher.favorites")} aria-pressed={activeFilter === "favorites"} onClick={() => setActiveFilter("favorites")}>
              <Star size={18} />
            </button>
            <button type="button" className="modelsw__rail-item" aria-label={t("modelSwitcher.allModels")} title={t("modelSwitcher.allModels")} aria-pressed={activeFilter === "all"} onClick={() => setActiveFilter("all")}>
              <List size={19} />
            </button>
            {providers.length > 0 && <span className="modelsw__rail-divider" aria-hidden="true" />}
            {providers.map((provider) => (
              <button key={provider.id} type="button" className="modelsw__rail-item" aria-label={provider.label} title={provider.label} aria-pressed={activeFilter === provider.id} onClick={() => setActiveFilter(provider.id)}>
                <ProviderMark provider={provider.id} label={provider.label} brandId={provider.brandId} />
              </button>
            ))}
            {sources.length > 0 && <span className="modelsw__rail-divider" aria-hidden="true" />}
            {sources.map(source => (
              <button key={source.id} type="button" className="modelsw__rail-item" aria-label={`${source.label} · Magpie`} title={`${source.label} · Magpie`} aria-pressed={activeFilter === source.id} onClick={() => setActiveFilter(source.id)}>
                <ProviderMark provider={source.upstream} label={source.label} brandId={source.brandId} />
              </button>
            ))}
          </nav>
          <div className="modelsw__catalog" role="listbox" aria-label={t("modelSwitcher.modelList")}>
            {models.length === 0 && <div className="modelsw__empty">{t("status.noModels")}</div>}
            {models.length > 0 && filtered.length === 0 && <div className="modelsw__empty">{activeFilter === "favorites" && !query ? t("modelSwitcher.noFavorites") : t("modelSwitcher.noMatches")}</div>}
            {groups.map((g) => (
            <div key={g.id} role="group" aria-label={g.label} className="modelsw__group">
              <div className="modelsw__group-label" role="presentation">{g.label}</div>
              {g.items.map((m) => {
                const favorite = favorites.has(m.ref);
                const meta = modelMeta(m, t);
                const favoriteLabel = t(favorite ? "modelSwitcher.removeFavorite" : "modelSwitcher.addFavorite", { model: m.model });
                return (
                <div className="modelsw__row" key={m.ref}>
                <button
                  type="button"
                  role="option"
                  aria-selected={m.current}
                  className={`modelsw__item ${m.current ? "modelsw__item--current" : ""}`}
                  onClick={() => pick(m)}
                >
                  <ProviderMark provider={m.provider} label={m.displayName?.trim() || providerLabel(m.provider, t)} brandId={m.brandId} />
                  <span className="modelsw__copy">
                    <span className="modelsw__model" title={m.model}>{m.modelDisplayName?.trim() || m.model}</span>
                    <span className="modelsw__meta" title={meta}>{meta}</span>
                  </span>
                  {m.contextWindow ? <span className="badge badge--neutral">{formatTokens(m.contextWindow)}</span> : null}
                  {m.vision && <span className="modelsw__capability" title={t("providerUI.image")}><Image size={13} aria-hidden="true" /><span>{t("providerUI.image")}</span></span>}
                  {m.current && <Check size={13} className="modelsw__check" />}
                </button>
                <button type="button" className={`modelsw__favorite${favorite ? " modelsw__favorite--active" : ""}`} aria-label={favoriteLabel} title={favoriteLabel} aria-pressed={favorite} onClick={() => toggleFavorite(m.ref)}>
                  <Star size={16} fill={favorite ? "currentColor" : "none"} />
                </button>
                </div>
              );})}
            </div>
          ))}
          </div>
        </div>
        {details && <div className="modelsw__details">{details}</div>}
        {onManage && <button className="modelsw__manage" type="button" onClick={() => { setOpen(false); onManage(); }}><Plus size={16} />{t("modelSwitcher.configureModels")}<ChevronRight size={15} /></button>}
      </AnchoredPopover>
    </div>
  );
}

export function normalizeModelInfo(model: ModelInfo): ModelInfo {
  return {
    ...model,
    provider: String(model.provider ?? ""),
    model: String(model.model ?? ""),
  };
}

function providerLabel(provider: string, t: ReturnType<typeof useT>): string {
  switch (provider) {
    case "deepseek":
    case "deepseek-flash":
    case "deepseek-pro":
      return t("settings.providerLabel.deepseek");
    default:
      return provider;
  }
}

function modelMeta(model: ModelInfo, t: ReturnType<typeof useT>): string {
  const provider = model.displayName?.trim() || providerLabel(model.provider, t);
  const name = model.modelDisplayName?.trim();
  return [name && name !== model.model ? model.model : null, provider, model.current ? t("modelSwitcher.currentModel") : null]
    .filter(Boolean).join(" · ");
}

function ProviderMark({ provider, label, brandId }: { provider: string; label: string; brandId?: string }) {
  const brandID = providerBrandID(provider, label, brandId);
  if (providerBrandIcons.has(brandID)) {
    const icon = `url(/provider-icons/${brandID}.svg)`;
    return <span className="modelsw__provider-icon" aria-hidden="true" style={{ maskImage: icon, WebkitMaskImage: icon }} />;
  }
  const monogram = label.trim().match(/[\p{L}\p{N}]/u)?.[0]?.toUpperCase() || "•";
  return <span className="modelsw__provider-monogram" aria-hidden="true">{monogram}</span>;
}
