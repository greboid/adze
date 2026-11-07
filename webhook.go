package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/greboid/adze/updater"
)

const (
	signedQueueSize = 100
	dangerQueueSize = 20
)

func NewHandler(secrets []string, imageUpdater updater.ImageUpdater) *Handler {
	h := &Handler{
		secrets: secrets,
		updater: imageUpdater,
		signed:  make(chan updateRequest, signedQueueSize),
		danger:  make(chan updateRequest, dangerQueueSize),
		pending: make(map[queueKey]struct{}),
	}
	go h.processUpdates()
	return h
}

func (h *Handler) Shutdown() {
	close(h.signed)
	close(h.danger)
}

func (h *Handler) processUpdates() {
	signed, danger := h.signed, h.danger
	for signed != nil || danger != nil {
		// Always take a signed request if one is waiting.
		select {
		case req, ok := <-signed:
			if !ok {
				signed = nil
				continue
			}
			h.run(req)
			continue
		default:
		}

		select {
		case req, ok := <-signed:
			if !ok {
				signed = nil
				continue
			}
			h.run(req)
		case req, ok := <-danger:
			if !ok {
				danger = nil
				continue
			}
			h.run(req)
		}
	}
}

func (h *Handler) run(req updateRequest) {
	// Clear the pending entry before starting, so a push that arrives while
	// this update is running gets its own run.
	h.mu.Lock()
	delete(h.pending, req.key())
	h.mu.Unlock()

	if err := h.updater.HandleUpdate(req.ctx, req.image, req.tag); err != nil {
		slog.Error("update failed", "image", req.image, "tag", req.tag, "error", err)
	}
}

type enqueueResult int

const (
	enqueued enqueueResult = iota
	alreadyQueued
	queueFull
)

func (h *Handler) enqueue(req updateRequest) enqueueResult {
	q := h.signed
	if req.danger {
		q = h.danger
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.pending[req.key()]; ok {
		return alreadyQueued
	}

	select {
	case q <- req:
		h.pending[req.key()] = struct{}{}
		return enqueued
	default:
		return queueFull
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

	switch h.enqueue(updateRequest{
		ctx:    context.WithoutCancel(r.Context()),
		image:  image,
		tag:    tag,
		danger: skipSignature,
	}) {
	case enqueued:
		w.WriteHeader(http.StatusAccepted)
	case alreadyQueued:
		slog.Info("update already queued, skipping", "image", image, "tag", tag, "source", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
	case queueFull:
		slog.Warn("update queue full, dropping webhook", "image", image, "tag", tag, "source", r.URL.Path)
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}
}
