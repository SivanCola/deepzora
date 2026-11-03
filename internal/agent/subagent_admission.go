package agent

// Tool read-only policy and resource admission are separate: configured hook
// commands may write even around a read-only tool, so they need the writer slot.
func (t *SubagentRunner) writableRun(spec ProfileExecSpec) bool {
	if !spec.Grant.ReadOnly {
		return true
	}
	if t.hooksForSession == nil {
		return false
	}
	return toolHooksMayMutateWorkspace(t.hooksForSession("subagent:admission"))
}
