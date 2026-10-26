// Goal budget helpers classify natural-language task text (English and
// Chinese) into delivery-intent categories. The delivery evidence gates and
// Goal budget selection consume it as a heuristic; it never gates permissions
// or whether writes are allowed.
package control

// Intent is the delivery expectation a task text implies.
type Intent uint8

const (
	// Conversation is chat with no host-observable work expected.
	Conversation Intent = iota
	// Advisory asks for explanation or advice rather than observable work.
	Advisory
	// ObservableRead expects host-observable read-only work (inspect, review).
	ObservableRead
	// Mutation expects workspace changes.
	Mutation
	// PersistentAction expects durable state kept across sessions.
	PersistentAction
)
