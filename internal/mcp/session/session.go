// Package session carries authenticated MCP request metadata across transport
// middleware and tool handlers.
package session

import "context"

type principalContextKey struct{}

// Principal is derived by the backend from the API key. Clients must never
// supply these fields directly.
type Principal struct {
	AccountID          string
	KeyID              string
	Plan               string
	Features           []string
	PlanCallsRemaining int
	CredentialHash     string
	BackendToken       string
}

// WithPrincipal attaches a backend-derived principal to a request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the authenticated principal, when present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// UsageEvent is the idempotent billing/audit event emitted by billable tool
// results. EventID must be unique for one completed operation.
type UsageEvent struct {
	EventID           string `json:"event_id"`
	AccountID         string `json:"account_id"`
	KeyID             string `json:"key_id"`
	Tool              string `json:"tool"`
	Result            string `json:"result"`
	SpecID            string `json:"spec_id"`
	SpecVersion       int    `json:"spec_version"`
	PlanningSessionID string `json:"planning_session_id,omitempty"`
}

// UsageRecorder persists one usage event. Implementations must be idempotent
// by EventID.
type UsageRecorder interface {
	RecordUsage(context.Context, UsageEvent) error
}

// UsageRecorderFunc adapts a function into a UsageRecorder.
type UsageRecorderFunc func(context.Context, UsageEvent) error

// RecordUsage records one usage event.
func (f UsageRecorderFunc) RecordUsage(ctx context.Context, event UsageEvent) error {
	return f(ctx, event)
}
