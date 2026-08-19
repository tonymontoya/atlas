// Package actor defines the one attribution identity Atlas write paths
// share: the acting subject and its display name. An Actor may be a
// human Operator, the Atlas system, or the Atlas Agent.
package actor

// Actor identifies the verified operator (or system component) an
// action is attributed to. The JSON field names are part of the agent
// request envelope wire contract (ADR-0018).
type Actor struct {
	Subject     string `json:"subject"`
	DisplayName string `json:"displayName"`
}
