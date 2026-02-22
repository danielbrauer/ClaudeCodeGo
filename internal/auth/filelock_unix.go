//go:build unix

package auth

import (
	"fmt"
	"os"
	"syscall"
)

// fileLock holds an open file with an exclusive flock.
type fileLock struct {
	f *os.File
}

// acquireFileLock opens (or creates) the file at path and attempts a
// non-blocking exclusive flock. Returns an error if the lock is held
// by another process.
func acquireFileLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	return &fileLock{f: f}, nil
}

// Unlock releases the flock and closes the file.
func (l *fileLock) Unlock() error {
	if l.f == nil {
		return nil
	}
	// Ignore flock unlock errors — closing the file releases the lock anyway.
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
