package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
)

func NewHandler(secrets []string, updater ImageUpdater) *Handler {
	h := &Handler{
		secrets: secrets,
		updater: updater,
		updates: make(chan updateRequest, 100),
	}
	go h.processUpdates()
	return h
}

func (h *Handler) Shutdown() {
	close(h.updates)
}

func (h *Handler) processUpdates() {
	for req := range h.updates {
		if err := h.updater.HandleUpdate(req.ctx, req.image, req.tag); err != nil {
			slog.Error("update failed", "image", req.image, "tag", req.tag, "error", err)
		}
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, false)
}

func (h *Handler) ServeHTTPDanger(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, true)
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request, skipSignature bool) {
	bodyReader := http.MaxBytesReader(w, r.Body, 500000)
	body, err := io.ReadAll(bodyReader)
	defer bodyReader.Close()
	if err != nil {
		slog.Warn("error reading body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if !skipSignature && !validateSignature(h.secrets, r.Header, body) {
		slog.Warn("webhook signature validation failed", "remote", r.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	payload := extractPayload(body, r.Header.Get("Content-Type"))
	image := extractImage(payload)
	if image == "" {
		slog.Warn("Payload doesn't match known image format.")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if !isRelevantEvent(payload) {
		slog.Info("ignoring event", "image", image, "source", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		return
	}

	tag := extractTag(payload)
	slog.Info("received webhook", "image", image, "tag", tag, "source", r.URL.Path)

	select {
	case h.updates <- updateRequest{
		ctx:   context.WithoutCancel(r.Context()),
		image: image,
		tag:   tag,
	}:
		w.WriteHeader(http.StatusAccepted)
	default:
		slog.Warn("update queue full, dropping webhook", "image", image, "tag", tag, "source", r.URL.Path)
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}
}
