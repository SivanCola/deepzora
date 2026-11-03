package control

// stricterApproval never widens a restriction. Missing values impose no cap;
// invalid persisted values normalize to read-only.
func stricterApproval(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if permissionPresetRank(b) < permissionPresetRank(a) {
		return normalizeToolApprovalMode(b)
	}
	return normalizeToolApprovalMode(a)
}

func (a *approvalManager) effectiveModeLocked() string {
	return stricterApproval(normalizeToolApprovalMode(a.toolApprovalMode), a.turnCeiling)
}

func (a *approvalManager) readOnlyCeilingLocked() bool {
	return a.turnCeiling != "" && a.effectiveModeLocked() == ToolApprovalReadOnly
}

func (a *approvalManager) persistentMode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return normalizeToolApprovalMode(a.toolApprovalMode)
}

func (c *Controller) effectiveApprovalMode() string { return c.approval.mode() }

// holdTeamCeiling changes the effective owner consumed by permissions and shell
// execution, leaving the user's persisted selection intact. The admitted turn
// owns this scope; Controller admission prevents a second foreground owner.
func (c *Controller) holdTeamCeiling(ceiling string) func() {
	if ceiling == "" {
		return func() {}
	}
	set := func(value string) {
		c.permissionMu.Lock()
		defer c.permissionMu.Unlock()
		c.permissionStateMu.Lock()
		defer c.permissionStateMu.Unlock()
		c.approval.mu.Lock()
		c.approval.turnCeiling = value
		c.approval.mu.Unlock()
		if c.subagentGate != nil {
			c.subagentGate.Update(c.effectiveApprovalMode())
		}
		c.refreshInteractiveGate()
		c.permissionRevision.Add(1)
	}
	set(normalizeToolApprovalMode(ceiling))
	return func() { set("") }
}
