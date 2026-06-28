package golitekit

import (
	"context"
	"testing"
)

func TestGenerateLogIDReturnsHexLength16(t *testing.T) {
	logID := generateLogID()
	if len(logID) != 16 {
		t.Fatalf("logID length = %d, want 16", len(logID))
	}
	for _, c := range logID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("logID contains non-hex character %q", c)
		}
	}
}

func TestEnsureLogIDCreatesAndReusesID(t *testing.T) {
	ctx := withContext(context.Background())

	first := EnsureLogID(ctx)
	if first == "" {
		t.Fatal("expected EnsureLogID to create a log ID")
	}

	second := EnsureLogID(ctx)
	if second != first {
		t.Fatalf("EnsureLogID second value = %q, want %q", second, first)
	}
}

func TestSetLogIDIgnoresEmpty(t *testing.T) {
	ctx := withContext(context.Background())
	SetLogID(ctx, "custom-log-id")

	SetLogID(ctx, "")

	if got := EnsureLogID(ctx); got != "custom-log-id" {
		t.Fatalf("logID = %q, want custom-log-id", got)
	}
}
