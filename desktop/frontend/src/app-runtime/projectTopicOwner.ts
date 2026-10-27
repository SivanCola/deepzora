import type { SessionOperationAuthority } from "./useResourceOperations";
import type { SessionSelector } from "../generated/desktopContract.generated";

export type TopicRenameTarget = { kind: "local"; topicId: string; selector: SessionSelector };
export type ProjectTopicPorts = {
  renameLocal: (selector: SessionSelector, title: string) => Promise<unknown>;
  markChanged: (update: (value: number) => number) => void;
  refreshTabs: (apply?: () => boolean, options?: { afterMutation?: boolean }) => Promise<readonly { id: string }[]>;
  syncActive: (rebuild: boolean) => Promise<unknown>;
};
export type ProjectRefreshInput = { activeTabId?: string; ports: ProjectTopicPorts };

export async function refreshProjectTopics(input: ProjectRefreshInput, authority: SessionOperationAuthority) {
  authority.checkpoint();
  input.ports.markChanged(value => value + 1);
  const tabs = await input.ports.refreshTabs(() => {
    try { authority.checkpoint(); return true; } catch { return false; }
  }, { afterMutation: true });
  authority.checkpoint();
  if (authority.ownsUI() && input.activeTabId && !tabs.some(tab => tab.id === input.activeTabId)) await input.ports.syncActive(false);
}

export async function renameProjectTopic(input: ProjectRefreshInput & { target: TopicRenameTarget; title: string }, authority: SessionOperationAuthority) {
  const { target, title, ports } = input;
  authority.checkpoint();
  await ports.renameLocal(target.selector, title);
  authority.checkpoint();
  await refreshProjectTopics(input, authority);
}
