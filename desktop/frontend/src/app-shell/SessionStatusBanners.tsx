import { ErrorMessage } from "../components/ErrorMessage";
import type { Translator } from "../lib/i18n";
import { UpdateBanner } from "../components/UpdateBanner";

export type SessionStatusBannersProps = {
  t: Translator;
  leaseBlocked: { tabId: string; message: string } | null;
  startupError: string | undefined;
  configWarnings: readonly string[];
  onOpenConfigFile: () => void;
  onReloadConfigFile: () => void;
  onDismissConfigWarnings: () => void;
  providerSetupNeeded: boolean;
  providerSetupInWelcome?: boolean;
  needsOnboarding: boolean | null;
  onConfigureProvider: () => void;
  updateChecksEnabled: boolean;
  onShowReleaseNotes: (latest: string) => void;
};

/** Presentation-only banner stack between the topic bar and the main pane. */
export function SessionStatusBanners(props: SessionStatusBannersProps) {
  const { t } = props;
  return (
    <>
      {props.leaseBlocked ? (
        <div className="banner banner--error">
          <span className="banner__msg"><ErrorMessage error={t("topbar.startupError", { msg: props.leaseBlocked.message })} /></span>
        </div>
      ) : props.startupError ? (
        <div className="banner banner--error">
          <span className="banner__msg"><ErrorMessage error={t("topbar.startupError", { msg: props.startupError })} /></span>
        </div>
      ) : null}
      {props.configWarnings.length > 0 && (
        <div className="banner banner--warning banner--actionable">
          <span className="banner__msg" title={props.configWarnings.join("\n")}>
            <ErrorMessage error={t("config.loadWarning", { msg: props.configWarnings.join("\n") })} />
          </span>
          <span className="banner__spacer" />
          <button type="button" className="btn btn--small" onClick={props.onOpenConfigFile}>
            {t("config.openConfig")}
          </button>
          <button type="button" className="btn btn--small" onClick={props.onReloadConfigFile}>
            {t("config.reloadConfig")}
          </button>
          <span className="banner__hint">{t("config.doctorHint")}</span>
          <button type="button" className="btn btn--small" onClick={props.onDismissConfigWarnings}>
            {t("updater.dismiss")}
          </button>
        </div>
      )}
      {props.providerSetupNeeded && !props.providerSetupInWelcome && (
        <div className="banner banner--warning banner--actionable">
          <span className="banner__msg">{t("onboarding.inlinePrompt")}</span>
          <span className="banner__spacer" />
          <button type="button" className="btn btn--small" onClick={props.onConfigureProvider}>
            {t("onboarding.configureProvider")}
          </button>
        </div>
      )}
      <UpdateBanner
        enabled={props.updateChecksEnabled}
        onShowReleaseNotes={props.onShowReleaseNotes}
      />
    </>
  );
}
