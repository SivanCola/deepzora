import { useMemo, type Dispatch, type SetStateAction } from "react";
import { useCommittedCommand } from "../lib/useCommittedCommand";
import {
  composerProfileForOwner,
  composerProfileFromMeta,
  composerProfileFromTab,
  composerProfileOwner,
  composerProfileOwnersConflict,
  defaultComposerProfile,
  displayedComposerProfileCollaborationMode,
  patchComposerProfile,
  updateUserPlanModeIntent,
  type ComposerProfile,
  type ComposerProfileField,
  type UserPlanModeIntents,
} from "../lib/composerProfile";
import type { Meta, TabMeta } from "../lib/types";

export type ComposerProfileProjectionInput = {
  activeTabId: string | undefined;
  activeTab: TabMeta | undefined;
  meta: Meta | null | undefined;
  profilesByTab: Record<string, ComposerProfile>;
  setProfilesByTab: Dispatch<SetStateAction<Record<string, ComposerProfile>>>;
  tabMetas: readonly TabMeta[];
  planIntentsRef: { current: UserPlanModeIntents };
};

/**
 * Owns the active composer profile projection (UI override over the backend
 * profile) and the profile patch commands: generic per-tab
 * patches and goal activation patches. Mode axis
 * changes stay in useComposerModeActions; this hook owns the profile record.
 */
export function useComposerProfileProjection(input: ComposerProfileProjectionInput) {
  const { activeTabId, activeTab, meta, profilesByTab, setProfilesByTab, tabMetas } = input;
  const activeOwner = composerProfileOwner(activeTab);
  const activeComposerProfile = activeTabId ? composerProfileForOwner(profilesByTab[activeTabId], activeOwner) : undefined;
  const backendActiveComposerProfile = useMemo(() => {
    if (meta && !composerProfileOwnersConflict(composerProfileOwner(meta), activeOwner)) {
      return composerProfileFromMeta(meta);
    }
    return composerProfileFromTab(activeTab);
  }, [activeOwner, activeTab, meta]);
  const composerProfile = activeTabId
    ? activeComposerProfile ?? backendActiveComposerProfile
    : defaultComposerProfile;
  const goal = composerProfile.goal;
  const collaborationMode = displayedComposerProfileCollaborationMode(composerProfile);
  const toolApprovalMode = composerProfile.toolApprovalMode;

  const patchActiveComposerProfile = useCommittedCommand((patch: Partial<Omit<ComposerProfile, "pending">>, pendingFields: ComposerProfileField[]) => {
    if (!activeTabId) return;
    setProfilesByTab((current) => patchComposerProfile(current, activeTabId, composerProfile, patch, pendingFields));
  });
  const patchComposerProfileForTab = useCommittedCommand((tabId: string, patch: Partial<Omit<ComposerProfile, "pending">>, pendingFields: ComposerProfileField[]) => {
    if (!tabId) return;
    setProfilesByTab((current) => {
      const tab = tabMetas.find((value) => value.id === tabId);
      const base = composerProfileForOwner(current[tabId], composerProfileOwner(tab)) ?? composerProfileFromTab(tab);
      return patchComposerProfile(current, tabId, base, patch, pendingFields);
    });
  });

  const patchActivatedGoalForTab = useCommittedCommand((tabId: string, nextGoal: string): void => {
    const trimmed = nextGoal.trim();
    patchComposerProfileForTab(tabId, {
      collaborationMode: trimmed ? "goal" : "normal",
      goalDraftMode: false,
      goal: trimmed,
    }, ["collaborationMode", "goal"]);
    input.planIntentsRef.current = updateUserPlanModeIntent(input.planIntentsRef.current, tabId, false);
  });

  return {
    composerProfile,
    goal,
    collaborationMode,
    toolApprovalMode,
    patchActiveComposerProfile,
    patchComposerProfileForTab,
    patchActivatedGoalForTab,
  };
}
