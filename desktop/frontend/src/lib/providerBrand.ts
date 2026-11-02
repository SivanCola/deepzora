import data from "./providerCatalog.generated.json";
import { providerBrandIcons } from "./providerBrandIcons";

const normalize = (value: string) => value.trim().toLowerCase();
const presetBrands = new Map(Object.entries(data.catalogs).map(([id, catalog]) => [id, catalog.brandId]));
const brandNames = new Map([
  ...Array.from(providerBrandIcons, id => [id, id] as const),
  ...Object.values(data.catalogs).flatMap(catalog => [
    [catalog.brandId, catalog.brandId] as const,
    [normalize(catalog.brandLabel), catalog.brandId] as const,
  ]),
]);
const connectionAliases = [...new Map([
  ...Array.from(brandNames.values(), id => [id, id] as const),
  ...presetBrands,
])].sort(([a], [b]) => b.length - a.length);

// Brand identity is independent of protocol, upstream model and icon availability.
// Older hosts omit brandId, so use the generated preset registry before guessing.
export function providerBrandID(provider: string, label: string, brandId?: string): string {
  if (brandId?.trim()) return normalize(brandId);
  const normalized = normalize(provider);
  const known = presetBrands.get(normalized) ?? brandNames.get(normalized);
  if (known) return known;
  const named = brandNames.get(normalize(label));
  if (named) return named;
  const alias = connectionAliases.find(([id]) =>
    normalized.startsWith(id) && /^[-_\s]/.test(normalized.slice(id.length)),
  );
  if (alias) return alias[1];

  // Preserve existing hints for custom connections outside the preset registry.
  if (normalized.includes("deepseek")) return "deepseek";
  if (normalized.includes("openai") || normalized.includes("gpt")) return "openai";
  if (normalized.includes("anthropic") || normalized.includes("claude")) return "anthropic";
  if (normalized.includes("google") || normalized.includes("gemini")) return "gemini";
  if (normalized.includes("glm") || normalized.includes("zhipu") || normalized.includes("zai")) return "zai";
  if (normalized.includes("minimax")) return "minimax";
  if (normalized.includes("qwen") || normalized.includes("dashscope")) return "qwen";
  if (normalized.includes("kimi") || normalized.includes("moonshot")) return "kimi";
  if (normalized.includes("xai") || normalized.includes("grok")) return "xai";
  return "";
}
