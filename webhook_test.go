package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testSecret = "testsecret"

// blockingUpdater records each update, and blocks on the first "blocker" update until
// release is closed so tests can fill the queues while the worker is busy.
type blockingUpdater struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once

	mu    sync.Mutex
	calls []string
}

func newBlockingUpdater() *blockingUpdater {
	return &blockingUpdater{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (u *blockingUpdater) HandleUpdate(_ context.Context, image string, _ string) error {
	u.mu.Lock()
	u.calls = append(u.calls, image)
	u.mu.Unlock()
	if image == "blocker" {
		u.once.Do(func() {
			close(u.started)
			<-u.release
		})
	}
	return nil
}

func (u *blockingUpdater) waitForCalls(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		u.mu.Lock()
		got := append([]string(nil), u.calls...)
		u.mu.Unlock()
		if len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	t.Fatalf("timed out waiting for %d calls, got %v", n, u.calls)
	return nil
}

func send(h *Handler, image string, danger bool) int {
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"image":"`+image+`"}`))
	rec := httptest.NewRecorder()
	if danger {
		h.ServeHTTPDanger(rec, req)
	} else {
		req.Header.Set("Authorization", "Bearer "+testSecret)
		h.ServeHTTP(rec, req)
	}
	return rec.Code
}

// startBlocked returns a handler whose worker is busy on the "blocker" image.
func startBlocked(t *testing.T) (*Handler, *blockingUpdater) {
	t.Helper()
	u := newBlockingUpdater()
	h := NewHandler([]string{testSecret}, u)
	t.Cleanup(h.Shutdown)
	if code := send(h, "blocker", false); code != http.StatusAccepted {
		t.Fatalf("blocker: got status %d", code)
	}
	<-u.started
	return h, u
}

func TestHandler_DeduplicatesPendingUpdates(t *testing.T) {
	h, u := startBlocked(t)

	for range 5 {
		if code := send(h, "app", false); code != http.StatusAccepted {
			t.Fatalf("got status %d, want %d", code, http.StatusAccepted)
		}
	}

	close(u.release)
	got := u.waitForCalls(t, 2)
	time.Sleep(50 * time.Millisecond)
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.calls) != 2 || got[1] != "app" {
		t.Errorf("calls = %v, want [blocker app]", u.calls)
	}
}

func TestHandler_RequeuesAfterUpdateStarts(t *testing.T) {
	h, u := startBlocked(t)

	// "blocker" is running, so a new push for the same image must be queued
	// rather than treated as a duplicate.
	if code := send(h, "blocker", false); code != http.StatusAccepted {
		t.Fatalf("got status %d", code)
	}

	close(u.release)
	got := u.waitForCalls(t, 2)
	if got[1] != "blocker" {
		t.Errorf("calls = %v, want [blocker blocker]", got)
	}
}

func TestHandler_SignedBeforeDanger(t *testing.T) {
	h, u := startBlocked(t)

	send(h, "danger1", true)
	send(h, "danger2", true)
	send(h, "signed1", false)
	send(h, "signed2", false)

	close(u.release)
	got := u.waitForCalls(t, 5)
	want := []string{"blocker", "signed1", "signed2", "danger1", "danger2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls = %v, want %v", got, want)
		}
	}
}

func TestHandler_DangerFloodDoesNotBlockSigned(t *testing.T) {
	h, u := startBlocked(t)
	defer close(u.release)

	for i := range dangerQueueSize {
		if code := send(h, "flood"+string(rune('a'+i)), true); code != http.StatusAccepted {
			t.Fatalf("danger %d: got status %d", i, code)
		}
	}
	if code := send(h, "overflow", true); code != http.StatusTooManyRequests {
		t.Errorf("danger overflow: got status %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := send(h, "app", false); code != http.StatusAccepted {
		t.Errorf("signed after danger flood: got status %d, want %d", code, http.StatusAccepted)
	}
}

func TestHandler_SignedNotDedupedAgainstDanger(t *testing.T) {
	h, u := startBlocked(t)

	send(h, "app", true)
	send(h, "app", false)

	close(u.release)
	got := u.waitForCalls(t, 3)
	if got[1] != "app" || got[2] != "app" {
		t.Errorf("calls = %v, want signed and danger app updates both run", got)
	}
}
