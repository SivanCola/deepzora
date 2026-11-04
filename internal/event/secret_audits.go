package event

import (
	"deepzora/internal/evidence"
	"deepzora/internal/secrets"
)

func (s *SecretSink) RuntimeStateChanged(snapshot RuntimeStateSnapshot) {
	PublishRuntimeState(s.Inner, secrets.Project(snapshot))
}
func (s *SecretSink) RecordAnchorSafetyAudit(a AnchorSafetyAudit) {
	RecordAnchorSafetyAudit(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordContractShadow(a ContractShadowAudit) {
	RecordContractShadow(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordMemoryRecall(a MemoryRecallAudit) {
	RecordMemoryRecall(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordDelegationAdmission(a DelegationAdmissionAudit) {
	RecordDelegationAdmission(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordOutcomeProgress(a evidence.OutcomeSample) {
	RecordOutcomeProgress(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordProtocolRecovery(a ProtocolRecoveryAudit) {
	RecordProtocolRecovery(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordDelegationAudit(a evidence.DelegationAudit) {
	RecordDelegationAudit(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordWorkspaceMutation(a WorkspaceMutation) {
	RecordWorkspaceMutation(s.Inner, secrets.Project(a))
}
func (s *SecretSink) RecordRunBudget(a RunBudgetSample) { RecordRunBudget(s.Inner, secrets.Project(a)) }
func (s *SecretSink) RecordSubagentLifecycle(a SubagentLifecycleInfo) {
	RecordSubagentLifecycle(s.Inner, secrets.Project(a))
}
