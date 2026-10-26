import { app, openExternal } from "../lib/bridge";
import { useCommittedCommand } from "../lib/useCommittedCommand";
export type ConfigWarningsReload = (warnings: string[], revision: number) => void;

/**
 * Owns the startup/session banner commands: config-file open/reload, provider setup navigation and the
 * release-notes link. Banner state (provider gate) lives on
 * the overlay store.
 */
export function useSessionBannerCommands(options: { reloadConfigWarnings: ConfigWarningsReload }) {
  const openConfigFile = useCommittedCommand(() => {
    void app.OpenUserConfigPath?.().catch(() => {});
  });

  const reloadConfigFile = useCommittedCommand(() => {
    void (async () => {
      try {
        const view = await app.ReloadUserConfig?.();
        options.reloadConfigWarnings(view?.configWarnings ?? [], view?.configWarningsRevision ?? 0);
      } catch {
        /* keep banner */
      }
    })();
  });

  const showReleaseNotes = useCommittedCommand((latest: string) => {
    const version = latest.replace(/^(?:desktop-)?v/, "");
    void openExternal(`https://github.com/SivanCola/deepzora/releases/tag/v${version}/`);
  });

  return { openConfigFile, reloadConfigFile, showReleaseNotes };
}
