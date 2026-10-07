package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gravitee-io-labs/gck/internal/logger"
)

// LockMode selects how a preload lock is held.
type LockMode int

const (
	// Shared is held by commands that push to the preload registry or
	// depend on it staying up. Any number of them can hold it together.
	Shared LockMode = iota
	// Exclusive is held by commands that stop the preload registry: garbage
	// collection and the teardown of the last cluster.
	Exclusive
)

// ErrPreloadBusy is returned when a lock is requested without waiting and
// another gck process holds a conflicting one.
var ErrPreloadBusy = errors.New("preload registry is in use by another gck command")

// PreloadLock is an advisory lock on $GCK_HOME/preload.lock. The kernel
// releases it when the holding process exits, so a crashed command never
// leaves a stale lock behind.
type PreloadLock struct {
	f *os.File
}

// TryLockPreload acquires the preload lock in the given mode, or returns
// ErrPreloadBusy at once if another process holds a conflicting lock.
func TryLockPreload(gckHome string, mode LockMode) (*PreloadLock, error) {
	return lockPreload(gckHome, mode, false)
}

// LockPreload acquires the preload lock in the given mode. When another
// process holds a conflicting lock, it shows waitMessage with a spinner and
// blocks until the lock is released.
func LockPreload(gckHome string, mode LockMode, waitMessage string) (*PreloadLock, error) {
	l, err := lockPreload(gckHome, mode, false)
	if !errors.Is(err, ErrPreloadBusy) {
		return l, err
	}
	err = logger.WithSpinner(waitMessage, func() error {
		l, err = lockPreload(gckHome, mode, true)
		return err
	})
	return l, err
}

func lockPreload(gckHome string, mode LockMode, wait bool) (*PreloadLock, error) {
	path := filepath.Join(gckHome, "preload.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	how := syscall.LOCK_SH
	if mode == Exclusive {
		how = syscall.LOCK_EX
	}
	if !wait {
		how |= syscall.LOCK_NB
	}

	for {
		err = syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrPreloadBusy
		}
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	return &PreloadLock{f: f}, nil
}

// Unlock releases the lock. It is safe to call on a nil lock.
func (l *PreloadLock) Unlock() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}
