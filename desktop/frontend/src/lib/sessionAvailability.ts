import type { Translator } from "./i18n";
import type { State } from "./useController";

export type SessionAvailability = {
  kind: "ready" | "loading" | "error" | "pending";
  source: "history" | "runtime";
  detail?: string;
};

type LocalSession = Pick<State, "meta" | "backendActivationPending" | "hydrating" | "hydrateError">;

/** Navigation, welcome and recovery must agree on the active source's readiness. */
export function projectSessionAvailability(input: { local?: LocalSession; empty?: boolean }): SessionAvailability {
  const { local } = input;
  if (input.empty) return { kind: "ready", source: "history" };
  if (local?.hydrateError) return { kind: "error", source: "history", detail: local.hydrateError };
  if (local?.meta?.startupErr) return { kind: "error", source: "runtime", detail: local.meta.startupErr };
  if (local?.hydrating) return { kind: "loading", source: "history" };
  const ready = local?.meta?.ready === true && !local.backendActivationPending
    && (!local.meta.runtime || local.meta.runtime.phase === "ready");
  return { kind: ready ? "ready" : "loading", source: "runtime" };
}

/** The reason a disabled composer states, or none when readiness alone explains it. */
export function submitBlockReason(availability: SessionAvailability, t: Translator): string | undefined {
  if (availability.kind === "pending") return t("sessionRecovery.importBeforeSend");
  if (availability.kind !== "ready" && availability.source !== "runtime") return t("sessionRecovery.sendAfterRecovery");
  return undefined;
}
