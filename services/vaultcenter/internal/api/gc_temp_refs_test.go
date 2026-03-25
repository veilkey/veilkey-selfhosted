package api

import (
	"testing"
	"time"

	"veilkey-vaultcenter/internal/db"
)

// ══════════════════════════════════════════════════════════════════
// Temp ref GC tests
//
// Verifies that the GC goroutine is actually started after unlock
// and that expired temp refs are cleaned up.
// ══════════════════════════════════════════════════════════════════

func TestGC_StartedAfterUnlock(t *testing.T) {
	// NewServer with KEK (unlocked) should have gcStop channel initialized
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	kek := make([]byte, 32)
	srv := NewServer(database, kek, []string{"127.0.0.1"})

	if srv.gcStop == nil {
		t.Error("gcStop channel must be initialized when server starts unlocked")
	}

	// Stop GC to clean up goroutine
	close(srv.gcStop)
}

func TestGC_NotStartedWhenLocked(t *testing.T) {
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// nil KEK = locked
	srv := NewServer(database, nil, []string{"127.0.0.1"})

	if srv.gcStop != nil {
		t.Error("gcStop channel must be nil when server starts locked")
	}
}

func TestGC_DeletesExpiredTempRefs(t *testing.T) {
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	expiredParts := db.RefParts{Family: "VK", Scope: db.RefScopeTemp, ID: "testgc01"}
	expiredAt := time.Now().UTC().Add(-1 * time.Hour)
	if err := database.SaveRefWithExpiry(expiredParts, "cipher1", 1, db.RefStatusTemp, expiredAt, "test-expired"); err != nil {
		t.Fatalf("failed to insert expired ref: %v", err)
	}

	futureParts := db.RefParts{Family: "VK", Scope: db.RefScopeTemp, ID: "testgc02"}
	futureAt := time.Now().UTC().Add(1 * time.Hour)
	if err := database.SaveRefWithExpiry(futureParts, "cipher2", 1, db.RefStatusTemp, futureAt, "test-valid"); err != nil {
		t.Fatalf("failed to insert valid ref: %v", err)
	}

	count, err := database.DeleteExpiredTempRefs()
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 expired ref deleted, got %d", count)
	}

	// Verify: expired ref gone, valid ref still present
	refs, err := database.ListRefs()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range refs {
		if r.RefCanonical == "VK:TEMP:testgc01" {
			t.Error("expired ref VK:TEMP:testgc01 should have been deleted")
		}
	}

	found := false
	for _, r := range refs {
		if r.RefCanonical == "VK:TEMP:testgc02" {
			found = true
		}
	}
	if !found {
		t.Error("valid ref VK:TEMP:testgc02 should still exist")
	}
}

func TestGC_GoroutineStopsOnClose(t *testing.T) {
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		StartTempRefGC(database, 50*time.Millisecond, stop)
		close(done)
	}()

	// Let GC run at least one tick
	time.Sleep(100 * time.Millisecond)

	// Stop and verify it exits
	close(stop)
	select {
	case <-done:
		// GC goroutine exited — success
	case <-time.After(2 * time.Second):
		t.Error("GC goroutine did not stop within 2 seconds after closing stop channel")
	}
}
