package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
)

func holdLock(t *testing.T, dir string) *os.File {
	t.Helper()
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func lockedUpdater(dir string) (*Updater, *mockComposeUpRunner) {
	lister := &mockContainerLister{
		containers: []container.Summary{{
			Image: "myapp:latest",
			Labels: map[string]string{
				api.ProjectLabel:    "myproject",
				api.WorkingDirLabel: dir,
			},
		}},
	}
	loader := &mockProjectLoader{project: &composetypes.Project{Services: composetypes.Services{"web": {Name: "web"}}}}
	up := &mockComposeUpRunner{}
	return NewUpdater(up, lister, loader, NoopNotifier{}, false, WithDirLocking(true)), up
}

func TestLockDir_WaitsForRelease(t *testing.T) {
	dir := t.TempDir()
	held := holdLock(t, dir)

	done := make(chan error, 1)
	go func() {
		f, err := lockDir(context.Background(), dir)
		if err == nil {
			f.Close()
		}
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("lock acquired while held: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	held.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lock not acquired after release")
	}
}

func TestLockDir_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	holdLock(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := lockDir(ctx, dir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestLockDir_Missing(t *testing.T) {
	f, err := lockDir(context.Background(), filepath.Join(t.TempDir(), "missing"))
	if f != nil || err != nil {
		t.Fatalf("expected no lock and no error, got %v, %v", f, err)
	}
}

func TestLockDir_NotDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := lockDir(context.Background(), path); err == nil {
		t.Fatal("expected error for non-directory")
	}
}

func TestHandleUpdate_LockHeldDuringUp(t *testing.T) {
	dir := t.TempDir()
	u, up := lockedUpdater(dir)
	up.onUp = func() {
		f, err := os.Open(dir)
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
			t.Errorf("expected directory to be locked during up, got %v", err)
		}
	}
	if err := u.HandleUpdate(context.Background(), "myapp:latest", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up.called.Load() {
		t.Fatal("expected Up to be called")
	}
	holdLock(t, dir)
}

func TestHandleUpdate_LockMissingDir(t *testing.T) {
	u, up := lockedUpdater(filepath.Join(t.TempDir(), "missing"))
	if err := u.HandleUpdate(context.Background(), "myapp:latest", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up.called.Load() {
		t.Error("expected Up to be called without a lock")
	}
}
