package session

import (
	"context"
	"errors"
	"testing"
)

func TestPrincipalRoundTrip(t *testing.T) {
	want := Principal{AccountID: "acct", KeyID: "key", Plan: "pro", Features: []string{"architect_plan"}}
	got, ok := PrincipalFromContext(WithPrincipal(context.Background(), want))
	if !ok {
		t.Fatal("principal missing from context")
	}
	if got.AccountID != want.AccountID || got.KeyID != want.KeyID || got.Plan != want.Plan {
		t.Fatalf("principal = %#v, want %#v", got, want)
	}
}

func TestPrincipalFromContextMissing(t *testing.T) {
	if _, ok := PrincipalFromContext(context.Background()); ok {
		t.Fatal("unexpected principal")
	}
}

func TestUsageRecorderFunc(t *testing.T) {
	wantErr := errors.New("backend unavailable")
	recorder := UsageRecorderFunc(func(_ context.Context, event UsageEvent) error {
		if event.EventID != "event" {
			t.Fatalf("event = %#v", event)
		}
		return wantErr
	})
	if err := recorder.RecordUsage(context.Background(), UsageEvent{EventID: "event"}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
