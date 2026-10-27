import React from "react";
import { createRoot } from "react-dom/client";
import { WorkspaceDockRegion } from "../src/app-shell/WorkspaceDockRegion";
import { PresentedFiles } from "../src/components/PresentedFiles";
import { LocaleProvider } from "../src/lib/i18n";
import { useActivityBarStore } from "../src/store/activityBar";
import { installDesktopHostStub } from "../src/__tests__/desktopHostStub";
import "../src/styles.css";

const file = (path: string) => ({ path, body: `NAVIGATION CONTENT ${path}`, size: 40, binary: false, truncated: false, mtimeUnix: 1 });
installDesktopHostStub({
  ListDirForTab: async () => [], SearchFileRefsForTab: async () => [],
  WorkspaceChanges: async () => ({ files: [], gitAvailable: true }), WorkspaceGitHistory: async () => [],
  ResolveWorkspacePathForTab: async (_tab: string, path: string) => path.startsWith("/") ? path : `/fixture/${path}`,
  ResolvePresentedPathForTab: async (_tab: string, _tool: string, path: string) => path.startsWith("/") ? path : `/fixture/${path}`,
  ReadFileForTab: async (_tab: string, path: string) => file(path),
  ReadPresentedFileForTab: async (_tab: string, _tool: string, path: string) => file(path),
  ReadPresentedFileSourceForTab: async (_tab: string, _tool: string, path: string) => file(path),
});
useActivityBarStore.setState({ workspaceRoot: "/fixture", tabs: [], activeTabId: null });

function Fixture() {
  const tabId = "local";
  return <LocaleProvider>
    <PresentedFiles tabId={tabId} files={[{ path: `${tabId}.txt`, toolCallId: "present", description: "Navigation regression" }]} />
    <WorkspaceDockRegion visible overlay={false} mode="files" creation={false} showContext={false}
      t={key => key} onPickEntry={() => {}} context={{} as never}
      workspace={{ open: true, tabId, cwd: "/fixture", maximized: false, onClose: () => {}, onToggleMaximized: () => {} }}
      workspaceRoot="/fixture" workspaceKey="fixture" />
  </LocaleProvider>;
}
createRoot(document.getElementById("root")!).render(<Fixture />);
