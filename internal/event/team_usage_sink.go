package event

type TeamUsageSink struct {
	AuditForwarder
	Owner TeamUsageAttribution
}

func NewTeamUsageSink(inner Sink, owner TeamUsageAttribution) *TeamUsageSink {
	return &TeamUsageSink{AuditForwarder: AuditForwarder{Inner: inner}, Owner: owner}
}
func (s *TeamUsageSink) stamp(e Event) Event       { owner := s.Owner; e.Team = &owner; return e }
func (s *TeamUsageSink) Emit(e Event)              { s.Inner.Emit(s.stamp(e)) }
func (s *TeamUsageSink) EmitChecked(e Event) error { return EmitChecked(s.Inner, s.stamp(e)) }
