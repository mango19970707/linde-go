package sessionrecorder

import "context"

type sessionIDContextKey struct{}

// WithSessionID returns a copy of ctx that carries the session ID.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDContextKey{}, sessionID)
}

// SessionIDFromContext extracts the session ID from ctx.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	sessionID, ok := ctx.Value(sessionIDContextKey{}).(string)
	if !ok || sessionID == "" {
		return "", false
	}
	return sessionID, true
}
