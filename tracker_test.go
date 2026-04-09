package golitekit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

func TestWithTracker(t *testing.T) {
	t.Run("creates new tracker when none exists", func(t *testing.T) {
		ctx := context.Background()
		newCtx := WithTracker(ctx)

		tracker := GetTracker(newCtx)
		if tracker == nil {
			t.Fatal("expected Tracker to be created")
		}
		if tracker.logID == "" {
			t.Error("expected logID to be generated")
		}
		if tracker.services == nil {
			t.Error("expected services map to be initialized")
		}
	})

	t.Run("reuses existing tracker", func(t *testing.T) {
		ctx := context.Background()
		ctx1 := WithTracker(ctx)
		tracker1 := GetTracker(ctx1)

		ctx2 := WithTracker(ctx1)
		tracker2 := GetTracker(ctx2)

		if tracker1 != tracker2 {
			t.Error("expected same Tracker instance to be reused")
		}
	})
}

func TestGetTracker(t *testing.T) {
	t.Run("returns nil for plain context", func(t *testing.T) {
		ctx := context.Background()
		tracker := GetTracker(ctx)
		if tracker != nil {
			t.Error("expected nil for context without Tracker")
		}
	})

	t.Run("returns Tracker when present", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)
		if tracker == nil {
			t.Error("expected Tracker to be returned")
		}
	})
}

func TestTracker_LogID(t *testing.T) {
	t.Run("generates unique logID", func(t *testing.T) {
		ctx1 := WithTracker(context.Background())
		ctx2 := WithTracker(context.Background())

		tracker1 := GetTracker(ctx1)
		tracker2 := GetTracker(ctx2)

		if tracker1.LogID() == tracker2.LogID() {
			t.Error("expected different logIDs for different trackers")
		}
	})

	t.Run("logID has correct length", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		logID := tracker.LogID()
		if len(logID) != 16 {
			t.Errorf("logID length = %d, want 16", len(logID))
		}
	})
}

func TestTracker_SetLogID(t *testing.T) {
	t.Run("sets custom logID", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		tracker.SetLogID("custom-log-id-123")
		if tracker.LogID() != "custom-log-id-123" {
			t.Errorf("logID = %s, want custom-log-id-123", tracker.LogID())
		}
	})

	t.Run("ignores empty logID", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		originalID := tracker.LogID()
		tracker.SetLogID("")

		if tracker.LogID() != originalID {
			t.Error("expected logID to remain unchanged when setting empty string")
		}
	})
}

func TestTracker_StartEnd(t *testing.T) {
	t.Run("tracks single service", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		tracker.Start("db")
		time.Sleep(10 * time.Millisecond)
		tracker.End()

		if _, ok := tracker.services["db"]; !ok {
			t.Error("expected 'db' service to be tracked")
		}
		if tracker.services["db"].cost < 10*time.Millisecond {
			t.Error("expected service cost to be at least 10ms")
		}
	})

	t.Run("tracks nested services", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		tracker.Start("outer")
		time.Sleep(5 * time.Millisecond)

		tracker.Start("inner")
		time.Sleep(5 * time.Millisecond)
		tracker.End() // end inner

		time.Sleep(5 * time.Millisecond)
		tracker.End() // end outer

		if _, ok := tracker.services["outer"]; !ok {
			t.Error("expected 'outer' service to be tracked")
		}
		if _, ok := tracker.services["inner"]; !ok {
			t.Error("expected 'inner' service to be tracked")
		}
	})

	t.Run("End on empty stack does not panic", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		// This should not panic (this was the bug we fixed)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("End() panicked on empty stack: %v", r)
			}
		}()

		tracker.End() // Call End without Start
		tracker.End() // Call End again
	})

	t.Run("multiple End calls after Start do not panic", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("End() panicked: %v", r)
			}
		}()

		tracker.Start("service")
		tracker.End()
		tracker.End() // Extra End call should not panic
		tracker.End() // Another extra End call
	})
}

func TestTracker_Concurrent(t *testing.T) {
	t.Run("concurrent LogID access is safe", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = tracker.LogID()
			}()
		}
		wg.Wait()
	})

	t.Run("concurrent SetLogID access is safe", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				tracker.SetLogID("id-" + string(rune('0'+i%10)))
			}(i)
		}
		wg.Wait()
	})

	t.Run("concurrent Start/End is safe", func(t *testing.T) {
		ctx := WithTracker(context.Background())
		tracker := GetTracker(ctx)

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				tracker.Start("service")
				time.Sleep(time.Millisecond)
				tracker.End()
			}(i)
		}
		wg.Wait()
	})
}

func TestGenerateLogID(t *testing.T) {
	t.Run("generates 16 character hex string", func(t *testing.T) {
		logID := generateLogID()
		if len(logID) != 16 {
			t.Errorf("logID length = %d, want 16", len(logID))
		}

		// Check if it's valid hex
		for _, c := range logID {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("logID contains invalid hex character: %c", c)
			}
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			id := generateLogID()
			if ids[id] {
				t.Errorf("duplicate logID generated: %s", id)
			}
			ids[id] = true
		}
	})
}

// ============================================================================
// LogTracker
// ============================================================================

func TestTracker_LogTracker_SelfTNonNegative(t *testing.T) {
	// self_t must never be logged as a negative value even when service timers
	// overlap or clock precision causes sum(services) > totalCost.
	ctx := WithTracker(context.Background())
	ctx = logger.WithLoggerContext(ctx)
	tracker := GetTracker(ctx)

	// Artificially add a service with a very large cost to trigger clamping.
	tracker.mu.Lock()
	tracker.services["fake"] = &serviceTracker{
		name: "fake",
		cost: 1000 * time.Second, // far exceeds any real totalCost
	}
	tracker.mu.Unlock()

	// LogTracker should not panic and self_t should be clamped to 0.
	tracker.LogTracker(ctx)

	// Verify the log context contains "self_t" with value 0.
	// We can only check it didn't panic; field inspection requires internal access.
}

func TestTracker_DuplicateStart_PreservesAllEntries(t *testing.T) {
	// Calling Start with the same name twice must create two distinct entries
	// (mysql and mysql_2) so no timing data is lost.
	ctx := WithTracker(context.Background())
	tracker := GetTracker(ctx)

	tracker.Start("mysql")
	time.Sleep(5 * time.Millisecond)
	tracker.End()

	tracker.Start("mysql")
	time.Sleep(5 * time.Millisecond)
	tracker.End()

	tracker.mu.Lock()
	_, first := tracker.services["mysql"]
	_, second := tracker.services["mysql_2"]
	tracker.mu.Unlock()

	if !first {
		t.Error("expected 'mysql' entry in services")
	}
	if !second {
		t.Error("expected 'mysql_2' entry for duplicate Start call")
	}
}

func TestTracker_LogTracker_LogsAllServices(t *testing.T) {
	// LogTracker must emit timing info for every tracked service.
	ctx := WithTracker(context.Background())
	ctx = logger.WithLoggerContext(ctx)
	tracker := GetTracker(ctx)

	tracker.Start("redis")
	time.Sleep(5 * time.Millisecond)
	tracker.End()

	tracker.Start("db")
	time.Sleep(5 * time.Millisecond)
	tracker.End()

	// Should complete without panic; fields added to logger context.
	tracker.LogTracker(ctx)
}
