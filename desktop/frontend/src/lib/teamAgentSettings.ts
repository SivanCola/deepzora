import type { TeamAgentSettings, TeamAgentView } from "../generated/desktopContract.generated";

// Copy only user-owned fields. Session bindings and attribution stay host-owned.
export const memberSettings = (member?: TeamAgentView): TeamAgentSettings => ({
  name: member?.name ?? "", role: member?.role ?? "", instructions: member?.instructions ?? "",
  avatar: member?.avatar ?? {}, initialModel: member?.initialModel ?? "", folder: member?.folder ?? "",
  permission: member?.permission || "read-only", main: member?.main ?? false,
  pinned: member?.pinned ?? false, hidden: member?.hidden ?? false, preset: member?.preset,
});
