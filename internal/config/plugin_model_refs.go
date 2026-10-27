package config

import (
	"strings"

	"deepzora/internal/extension/protocol"
)

// RemovePluginModelRefs moves every model ref owned by pluginID to the first
// remaining configured provider, or clears it when none is left, and reports
// whether anything changed. Plugin refs resolve outside the config catalog, so
// a removed plugin would otherwise strand them and fail the next boot.
func (c *Config) RemovePluginModelRefs(pluginID string) bool {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return false
	}
	return c.retargetModelRefs(func(ref string) bool {
		return protocol.PluginRefOwner(strings.TrimSpace(ref)) == pluginID
	}, c.providerRemovalFallback(""))
}
