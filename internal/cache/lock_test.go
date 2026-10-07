package cache

import (
	"errors"
	"testing"
	"time"
)

func TestPreloadLock(t *testing.T) {
	home := t.TempDir()

	// Each call opens its own file description, so locks taken by this
	// process conflict with each other the way two gck processes would.
	a, err := TryLockPreload(home, Shared)
	if err != nil {
		t.Fatalf("first shared lock: %v", err)
	}
	b, err := TryLockPreload(home, Shared)
	if err != nil {
		t.Fatalf("second shared lock: %v", err)
	}
	if _, err := TryLockPreload(home, Exclusive); !errors.Is(err, ErrPreloadBusy) {
		t.Fatalf("exclusive while shared held: got %v, want ErrPreloadBusy", err)
	}

	a.Unlock()
	b.Unlock()
	b.Unlock() // releasing twice is harmless

	ex, err := TryLockPreload(home, Exclusive)
	if err != nil {
		t.Fatalf("exclusive once released: %v", err)
	}
	if _, err := TryLockPreload(home, Shared); !errors.Is(err, ErrPreloadBusy) {
		t.Fatalf("shared while exclusive held: got %v, want ErrPreloadBusy", err)
	}
	ex.Unlock()

	var none *PreloadLock
	none.Unlock()
}

func TestLockPreloadWaitsForRelease(t *testing.T) {
	home := t.TempDir()
	ex, err := TryLockPreload(home, Exclusive)
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan error, 1)
	go func() {
		l, err := LockPreload(home, Shared, "waiting")
		l.Unlock()
		acquired <- err
	}()

	select {
	case err := <-acquired:
		t.Fatalf("shared lock acquired while exclusive held (err %v)", err)
	case <-time.After(200 * time.Millisecond):
	}

	ex.Unlock()
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shared lock not acquired after exclusive release")
	}
}
