package sessionrecorder

import (
	"context"
	"testing"
)

func TestContextSessionIDRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := WithSessionID(context.Background(), "session-123")

	got, ok := SessionIDFromContext(ctx)
	if !ok {
		t.Fatal("SessionIDFromContext() ok = false")
	}
	if got != "session-123" {
		t.Fatalf("SessionIDFromContext() = %q", got)
	}
}
