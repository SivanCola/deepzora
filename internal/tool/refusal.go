package tool

// Refusal is a stable host refusal identity, independent of transport or UI.
type Refusal struct {
	Code    string
	Message string
}

func (r Refusal) Error() string       { return r.Code + ": " + r.Message }
func (r Refusal) RefusalCode() string { return r.Code }
