import type { GeneratedDesktopCommands } from "../generated/desktopContract.generated";

export type TeamHostBindings = Pick<GeneratedDesktopCommands,
  "BrowserTeamAgentForTab" | "GetTeam" | "OpenTeamAgent" | "SaveTeamAgent" | "SaveTeamGroup"
  | "ReadTeamGroupHistory" | "SendTeamGroupMessage" | "ContinueTeamGroup" | "StopTeamSeat" | "StopTeamGroup" | "PrepareTeamAgentSession" | "DeleteTeamTarget" | "ReorderTeam"
  | "ReadTeamGroupDraft" | "SaveTeamGroupDraft" | "MarkTeamGroupRead"
  | "GetTeamMemory" | "SaveTeamMemory" | "GetTeamMemoryReviewStatus" | "SaveTeamMemoryPreferences" | "GetTeamSchedules" | "SaveTeamSchedule" | "RunTeamSchedule">;

// This increment has no browser-only Team engine. The production proxy forwards
// the generated contract to Go; the browser preview reports the absent host.
export function makeUnavailableTeamHostMock(): TeamHostBindings {
  const unavailable = async (): Promise<never> => { throw new Error("Team execution requires the local Desktop host"); };
  return {
    BrowserTeamAgentForTab: async () => "",
    GetTeam: async () => ({ roster: { schemaVersion: 1, revision: 0, agents: [], groups: [], retiredAgents: [], retiredGroups: [],prefs:{memoryReview:false} }, writable: false, seats: [], groupStatus: [],agentActivity:[] }),
    GetTeamMemory: unavailable,
    SaveTeamMemory: unavailable,
    GetTeamMemoryReviewStatus: unavailable,
    SaveTeamMemoryPreferences: unavailable,
    GetTeamSchedules: unavailable,
    SaveTeamSchedule: unavailable,
    RunTeamSchedule: unavailable,
    OpenTeamAgent: unavailable,
    SaveTeamAgent: unavailable,
    SaveTeamGroup: unavailable,
    ReadTeamGroupHistory: unavailable,
    SendTeamGroupMessage: unavailable,
    ContinueTeamGroup: unavailable,
    StopTeamSeat: unavailable,
    StopTeamGroup: unavailable,
    PrepareTeamAgentSession: unavailable,
    DeleteTeamTarget: unavailable,
    ReorderTeam: unavailable,
    ReadTeamGroupDraft: unavailable,
    SaveTeamGroupDraft: unavailable,
    MarkTeamGroupRead: unavailable,
  };
}
