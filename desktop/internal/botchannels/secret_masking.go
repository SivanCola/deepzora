package botchannels

import (
	"deepzora/internal/botcredentials"
	"deepzora/internal/secrets"
)

func credentialValues(entries []Saved) []secrets.Labeled {
	values := make([]secrets.Labeled, 0, len(entries))
	for _, entry := range entries {
		values = append(values, secrets.Labeled{Label: "BOT_" + entry.ID, Value: entry.Secret})
	}
	return values
}

// InstallSecretValues shares the same source with CLI/ACP runtime startup.
func (s Store) InstallSecretValues() func() { return botcredentials.Install(s.Dir) }
