package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type WebhookNotifier struct {
	url    string
	secret string
	client *http.Client
}

func NewWebhookNotifier(url, secret string) *WebhookNotifier {
	return &WebhookNotifier{
		url:    url,
		secret: secret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *WebhookNotifier) NotifyPending(ctx context.Context, image string, target string, dir string) {
	n.send(ctx, notificationPayload{
		Image:  image,
		Target: target,
		Dir:    dir,
		Status: "pending",
	})
}

func (n *WebhookNotifier) NotifyResult(ctx context.Context, image string, target string, dir string, err error) {
	payload := notificationPayload{
		Image:  image,
		Target: target,
		Dir:    dir,
		Status: "success",
	}
	if err != nil {
		payload.Status = "failure"
		payload.Error = err.Error()
	}
	n.send(ctx, payload)
}

func (n *WebhookNotifier) send(ctx context.Context, payload notificationPayload) {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal notification payload", "image", payload.Image, "target", payload.Target, "dir", payload.Dir, "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create notification request", "image", payload.Image, "target", payload.Target, "dir", payload.Dir, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	if n.secret != "" {
		mac := hmac.New(sha256.New, []byte(n.secret))
		mac.Write(body)
		req.Header.Set("X-Adze-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	slog.Debug("sending notification webhook", "image", payload.Image, "target", payload.Target, "dir", payload.Dir, "status", payload.Status, "url", n.url)

	resp, err := n.client.Do(req)
	if err != nil {
		slog.Error("failed to send notification", "image", payload.Image, "target", payload.Target, "dir", payload.Dir, "url", n.url, "error", err)
		return
	}
	resp.Body.Close()

	slog.Debug("notification sent", "image", payload.Image, "target", payload.Target, "dir", payload.Dir, "status", payload.Status, "url", n.url, "response_code", resp.StatusCode)

	if resp.StatusCode >= 300 {
		slog.Warn("notification webhook returned non-success status", "image", payload.Image, "target", payload.Target, "dir", payload.Dir, "url", n.url, "status", resp.StatusCode)
	}
}
