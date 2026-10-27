package boot

import "deepzora/internal/config"

func runtimeConfigHostClassification() []configTypeClassification {
	return []configTypeClassification{
		classifyConfigType[config.DesktopConfig](configFieldClasses{
			runtime: "ProviderAccess",
			excluded: "Language LayoutStyle Theme ThemeStyle TerminalTheme ExternalOpener CloseBehavior " +
				"StatusBarStyle StatusBarItems DefaultToolApprovalMode CheckUpdates " +
				"Telemetry Metrics SessionExperience ConversationWidth",
		}),
		classifyConfigType[config.UIConfig](configFieldClasses{
			excluded: "Theme ThemeStyle ShowReasoning ShowTurnUsage CursorShape",
		}),
		classifyConfigType[config.CLIConfig](configFieldClasses{excluded: ""}),
		classifyConfigType[config.TelemetryConfig](configFieldClasses{excluded: "CLIMetrics"}),
		classifyConfigType[config.NotificationsConfig](configFieldClasses{excluded: "Enabled TurnDone ApprovalRequest AskRequest"}),
		classifyConfigType[config.StatuslineConfig](configFieldClasses{excluded: "Command"}),
	}
}
