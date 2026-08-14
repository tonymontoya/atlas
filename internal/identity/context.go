package identity

import (
	"context"
)

type contextKey struct{}

// WithContext returns a context carrying the verified Identity.
func WithContext(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the verified Identity in the context, if any.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(contextKey{}).(Identity)
	return id, ok
}
