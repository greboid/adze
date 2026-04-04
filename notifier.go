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

func (n *WebhookNotifier) NotifyPending(ctx context.Context, image string, target string) {
	n.send(ctx, notificationPayload{
		Image:  image,
		Target: target,
		Status: "pending",
	})
}

func (n *WebhookNotifier) NotifyResult(ctx context.Context, image string, target string, err error) {
	payload := notificationPayload{
		Image:  image,
		Target: target,
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
		slog.Error("failed to marshal notification payload", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create notification request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	if n.secret != "" {
		mac := hmac.New(sha256.New, []byte(n.secret))
		mac.Write(body)
		req.Header.Set("X-Adze-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		slog.Error("failed to send notification", "url", n.url, "error", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		slog.Error("notification webhook returned non-success status", "url", n.url, "status", resp.StatusCode)
	}
}
