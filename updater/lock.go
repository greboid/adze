package updater

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"syscall"
	"time"
)

// lockDir waits until ctx ends for an exclusive flock on dir. Closing the file releases the lock.
// A missing dir returns no lock.
func lockDir(ctx context.Context, dir string) (*os.File, error) {
	f, err := os.Open(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.IsDir() {
		err = fmt.Errorf("%s is not a directory", dir)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	logged := false
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			if err != nil {
				f.Close()
				return nil, err
			}
			return f, nil
		}
		if !logged {
			slog.Info("waiting for project directory lock", "dir", dir)
			logged = true
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
