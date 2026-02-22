//go:build !unix

package auth

import (
	"os"
)

// fileLock is a no-op on non-Unix platforms.
type fileLock struct {
	f *os.File
}

// acquireFileLock is a best-effort lock on non-Unix platforms.
// It opens the file but does not acquire an OS-level lock.
func acquireFileLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	return &fileLock{f: f}, nil
}

// Unlock closes the lock file.
func (l *fileLock) Unlock() error {
	if l.f == nil {
		return nil
	}
	return l.f.Close()
}
