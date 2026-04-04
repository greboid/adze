package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	n.NotifyPending(context.Background(), "myapp:latest", "myproject")

	if received.payload.Image != "myapp:latest" {
		t.Errorf("expected image %q, got %q", "myapp:latest", received.payload.Image)
	}
	if received.payload.Target != "myproject" {
		t.Errorf("expected target %q, got %q", "myproject", received.payload.Target)
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
	n.NotifyResult(context.Background(), "myapp:latest", "myproject", nil)

	if received.Image != "myapp:latest" {
		t.Errorf("expected image %q, got %q", "myapp:latest", received.Image)
	}
	if received.Target != "myproject" {
		t.Errorf("expected target %q, got %q", "myproject", received.Target)
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
	n.NotifyResult(context.Background(), "myapp:latest", "myproject", context.DeadlineExceeded)

	if received.Status != "failure" {
		t.Errorf("expected status %q, got %q", "failure", received.Status)
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
	n.NotifyPending(context.Background(), "myapp", "proj")

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
	n.NotifyPending(context.Background(), "myapp", "proj")

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
	n.NotifyPending(context.Background(), "myapp", "proj")

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
	n.NotifyResult(context.Background(), "myapp", "proj", nil)
}

func TestWebhookNotifier_InvalidURL(t *testing.T) {
	n := NewWebhookNotifier("http://127.0.0.1:0/nowhere", "")
	// Should not panic, just log
	n.NotifyPending(context.Background(), "myapp", "proj")
}

func TestNoopNotifier(t *testing.T) {
	n := noopNotifier{}
	// Should not panic
	n.NotifyPending(context.Background(), "myapp", "proj")
	n.NotifyResult(context.Background(), "myapp", "proj", nil)
	n.NotifyResult(context.Background(), "myapp", "proj", context.DeadlineExceeded)
}
