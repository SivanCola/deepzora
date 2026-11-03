import type { GeneratedDesktopCommands } from "../generated/desktopContract.generated";

export type TeamHostBindings = Pick<GeneratedDesktopCommands,
  "BrowserTeamAgentForTab" | "GetTeam" | "OpenTeamAgent" | "SaveTeamAgent" | "SaveTeamGroup">;

// This increment has no browser-only Team engine. The production proxy forwards
// the generated contract to Go; the browser preview reports the absent host.
export function makeUnavailableTeamHostMock(): TeamHostBindings {
  const unavailable = async (): Promise<never> => { throw new Error("Team execution requires the local Desktop host"); };
  return {
    BrowserTeamAgentForTab: async () => "",
    GetTeam: async () => ({ roster: { schemaVersion: 1, revision: 0, agents: [], groups: [] }, writable: false, seats: [] }),
    OpenTeamAgent: unavailable,
    SaveTeamAgent: unavailable,
    SaveTeamGroup: unavailable,
  };
}
