package event

type TeamUsageAttribution struct {
	AgentID string `json:"agentId"`
	GroupID string `json:"groupId,omitempty"`
	SeatID  string `json:"seatId,omitempty"`
}
