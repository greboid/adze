package updater

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookNotifier_NotifyPending(t *testing.T) {
	var received struct {
		payload notificationPayload
		header  string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received.payload)
		received.header = r.Header.Get("X-Adze-Signature")
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "test-secret")
	n.NotifyPending(context.Background(), "myapp:latest", "myproject", "/opt/myproject")

	if received.payload.Image != "myapp:latest" {
		t.Errorf("expected image %q, got %q", "myapp:latest", received.payload.Image)
	}
	if received.payload.Target != "myproject" {
		t.Errorf("expected target %q, got %q", "myproject", received.payload.Target)
	}
	if received.payload.Dir != "/opt/myproject" {
		t.Errorf("expected dir %q, got %q", "/opt/myproject", received.payload.Dir)
	}
	if received.payload.Status != "pending" {
		t.Errorf("expected status %q, got %q", "pending", received.payload.Status)
	}
	if received.payload.Error != "" {
		t.Errorf("expected no error, got %q", received.payload.Error)
	}
	if received.header == "" {
		t.Error("expected signature header to be set")
	}
}

func TestWebhookNotifier_NotifyResult_Success(t *testing.T) {
	var received notificationPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "test-secret")
	n.NotifyResult(context.Background(), "myapp:latest", "myproject", "/opt/myproject", nil)

	if received.Image != "myapp:latest" {
		t.Errorf("expected image %q, got %q", "myapp:latest", received.Image)
	}
	if received.Target != "myproject" {
		t.Errorf("expected target %q, got %q", "myproject", received.Target)
	}
	if received.Dir != "/opt/myproject" {
		t.Errorf("expected dir %q, got %q", "/opt/myproject", received.Dir)
	}
	if received.Status != "success" {
		t.Errorf("expected status %q, got %q", "success", received.Status)
	}
	if received.Error != "" {
		t.Errorf("expected no error, got %q", received.Error)
	}
}

func TestWebhookNotifier_NotifyResult_Failure(t *testing.T) {
	var received notificationPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "test-secret")
	n.NotifyResult(context.Background(), "myapp:latest", "myproject", "/opt/myproject", context.DeadlineExceeded)

	if received.Status != "failure" {
		t.Errorf("expected status %q, got %q", "failure", received.Status)
	}
	if received.Dir != "/opt/myproject" {
		t.Errorf("expected dir %q, got %q", "/opt/myproject", received.Dir)
	}
	if received.Error != "context deadline exceeded" {
		t.Errorf("expected error %q, got %q", "context deadline exceeded", received.Error)
	}
}

func TestWebhookNotifier_SignatureWithSecret(t *testing.T) {
	var sigHeader string
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-Adze-Signature")
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	secret := "my-hmac-secret"
	n := NewWebhookNotifier(srv.URL, secret)
	n.NotifyPending(context.Background(), "myapp", "proj", "/dir")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if sigHeader != expected {
		t.Errorf("expected signature %q, got %q", expected, sigHeader)
	}
}

func TestWebhookNotifier_NoSignatureWhenEmptySecret(t *testing.T) {
	var sigHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-Adze-Signature")
		io.ReadAll(r.Body)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "")
	n.NotifyPending(context.Background(), "myapp", "proj", "")

	if sigHeader != "" {
		t.Errorf("expected no signature header, got %q", sigHeader)
	}
}

func TestWebhookNotifier_ContentTypeJSON(t *testing.T) {
	var contentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		io.ReadAll(r.Body)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "")
	n.NotifyPending(context.Background(), "myapp", "proj", "")

	if contentType != "application/json" {
		t.Errorf("expected content type %q, got %q", "application/json", contentType)
	}
}

func TestWebhookNotifier_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.ReadAll(r.Body)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, "")
	// Should not panic, just log
	n.NotifyResult(context.Background(), "myapp", "proj", "", nil)
}

func TestWebhookNotifier_InvalidURL(t *testing.T) {
	n := NewWebhookNotifier("http://127.0.0.1:0/nowhere", "")
	// Should not panic, just log
	n.NotifyPending(context.Background(), "myapp", "proj", "")
}

func TestNoopNotifier(t *testing.T) {
	n := NoopNotifier{}
	// Should not panic
	n.NotifyPending(context.Background(), "myapp", "proj", "")
	n.NotifyResult(context.Background(), "myapp", "proj", "", nil)
	n.NotifyResult(context.Background(), "myapp", "proj", "", context.DeadlineExceeded)
}

func TestRedactURL(t *testing.T) {
	tests := map[string]string{
		"https://discord.com/api/webhooks/123/supersecret":     "https://discord.com",
		"https://user:pass@example.com/hook?token=supersecret": "https://example.com",
		"http://ntfy.local:8080/topic":                         "http://ntfy.local:8080",
		"not a url":                                            "[redacted]",
		"://bad":                                               "[redacted]",
	}
	for in, want := range tests {
		if got := RedactURL(in); got != want {
			t.Errorf("RedactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWebhookNotifier_ErrorsDoNotContainURL(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(old)

	n := NewWebhookNotifier("http://127.0.0.1:1/hook/supersecret", "")
	n.NotifyPending(context.Background(), "img", "target", "")

	if strings.Contains(buf.String(), "supersecret") {
		t.Errorf("log output contains secret URL path:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "failed to send notification") {
		t.Errorf("expected send failure to be logged, got:\n%s", buf.String())
	}
}
