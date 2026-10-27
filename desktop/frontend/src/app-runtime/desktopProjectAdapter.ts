import { app } from "../lib/bridge";
import type { SessionSelector } from "../generated/desktopContract.generated";

export const desktopProjectAdapter = {
  renameLocal: (selector: SessionSelector, title: string) => app.RenameSessionTarget(selector, title),
};
