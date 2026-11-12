// Parsing of install_source plan JSON for the plugin settings page.
import type { PluginCompatibilityIssue } from "./types";

export type PluginRuntimePlan = {
  command?: string;
  args?: string[];
  intercepts?: string[];
  replaces?: string[];
  capabilities?: string[];
  fullTrust?: boolean;
};

export type PluginInstallPlanAction = {
  changes?: string[];
  action?: string;
  kind?: string;
  name?: string;
  source?: string;
  status?: string;
  message?: string;
  error?: string;
  compatibility?: string;
  mappedCapabilities?: string[];
  skippedCapabilities?: PluginCompatibilityIssue[];
  runtime?: PluginRuntimePlan;
  agentCount?: number;
  skillCount?: number;
  commandCount?: number;
  hookCount?: number;
  toolCount?: number;
};

export type PluginInstallPlanView = {
  planId?: string;
  activationWarning?: string;
  raw: string;
  ok?: boolean;
  status?: string;
  name?: string;
  actions: PluginInstallPlanAction[];
  warnings: string[];
  error?: string;
};


export function parsePluginInstallPlan(raw: string): PluginInstallPlanView {
	try {
		const value = JSON.parse(raw) as Record<string, unknown>;
		const actions = (Array.isArray(value.actions) ? value.actions : []).flatMap((action) => {
			if (!action || typeof action !== "object") return [];
			const item = action as Record<string, unknown>;
			return [{
				action: stringValue(item.action),
				kind: stringValue(item.kind),
				name: stringValue(item.name),
				source: stringValue(item.source),
				status: stringValue(item.status),
				message: stringValue(item.message),
				error: stringValue(item.error),
				compatibility: stringValue(item.compatibility),
				mappedCapabilities: (Array.isArray(item.mappedCapabilities) ? item.mappedCapabilities : []).filter((value): value is string => typeof value === "string"),
				skippedCapabilities: (Array.isArray(item.skippedCapabilities) ? item.skippedCapabilities : []) as PluginCompatibilityIssue[],
				runtime: parsePluginRuntimePlan(item.runtime),
				changes: marketPlanChanges(item),
				agentCount: numericValue(item.agentCount), skillCount: numericValue(item.skillCount), commandCount: numericValue(item.commandCount), hookCount: numericValue(item.hookCount), toolCount: numericValue(item.toolCount),
			}];
		});
		return {
			raw,
			ok: typeof value.ok === "boolean" ? value.ok : undefined,
			planId: stringValue(value.planId),
			activationWarning: stringValue(value.activationWarning),
			status: stringValue(value.status),
			name: stringValue(value.name),
			actions,
			warnings: (Array.isArray(value.warnings) ? value.warnings : []).flatMap((warning) => typeof warning === "string" ? [warning] : []),
			error: stringValue(value.error),
		};
	} catch {
		return { raw, actions: [], warnings: [] };
	}
}

export function marketPlanChanges(item:Record<string,unknown>):string[] {
 const c=item.catalog as {execution?:string[];previousExecution?:string[];requirements?:{namespace:string;kind:string;id:string}[];previous?:{version:string};previousCapabilities?:Record<string,number|boolean>;capabilities?:Record<string,number|boolean>}|undefined;
 if(!c)return [];
 const lines=(Array.isArray(item.riskReasons)?item.riskReasons:[]).filter((x):x is string=>typeof x==="string");
 for(const s of c.execution||[])lines.push((c.previous && !c.previousExecution?.includes(s)?"+ ":"")+s);
 for(const s of c.previousExecution||[])if(!c.execution?.includes(s))lines.push("− "+s);
 for(const r of c.requirements||[])lines.push("requires: "+[r.namespace,r.kind,r.id].join("/"));
 if(c.previous){lines.unshift(c.previous.version+" → "+item.version);
 for(const [k,v] of Object.entries(c.capabilities||{}))if(c.previousCapabilities?.[k]!==v)lines.push(k+": "+(c.previousCapabilities?.[k]??0)+" → "+v)}
 return lines;
}
export function numericValue(value: unknown): number | undefined {
	return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

export function stringValue(value: unknown): string | undefined {
	return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

// parsePluginRuntimePlan extracts the FULL TRUST runtime block a plugin
// install plan carries (installsource.RuntimePlanInfo). Anything malformed
// simply drops out — the risk UI is additive and must never break planning.
export function parsePluginRuntimePlan(value: unknown): PluginRuntimePlan | undefined {
	if (!value || typeof value !== "object") return undefined;
	const item = value as Record<string, unknown>;
	const command = stringValue(item.command);
	if (!command) return undefined;
	const list = (v: unknown): string[] => (Array.isArray(v) ? v : []).filter((entry): entry is string => typeof entry === "string" && entry.trim().length > 0);
	return {
		command,
		args: list(item.args),
		intercepts: list(item.intercepts),
		replaces: list(item.replaces),
		capabilities: list(item.capabilities),
		fullTrust: item.fullTrust === true,
	};
}
