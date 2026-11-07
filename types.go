package main

import (
	"context"
	"sync"

	"github.com/greboid/adze/updater"
)

type webhookPayload struct {
	Image   string `json:"image"`
	Package struct {
		Owner struct {
			Login   string `json:"login"`
			HTMLURL string `json:"html_url"`
		} `json:"owner"`
		Type           string `json:"type"`
		PackageType    string `json:"package_type"`
		Name           string `json:"name"`
		Version        string `json:"version"`
		PackageVersion struct {
			ContainerMetadata struct {
				Tag struct {
					Name string `json:"name"`
				} `json:"tag"`
			} `json:"container_metadata"`
		} `json:"package_version"`
	} `json:"package"`
	Events []struct {
		Action string `json:"action"`
		Target struct {
			Repository string `json:"repository"`
			MediaType  string `json:"mediaType"`
			Tag        string `json:"tag"`
		} `json:"target"`
		Request struct {
			Host string `json:"host"`
		} `json:"request"`
	} `json:"events"`
}

type updateRequest struct {
	ctx    context.Context
	image  string
	tag    string
	danger bool
}

func (r updateRequest) key() queueKey {
	return queueKey{danger: r.danger, image: r.image, tag: r.tag}
}

type queueKey struct {
	danger bool
	image  string
	tag    string
}

type Handler struct {
	secrets []string
	updater updater.ImageUpdater

	// signed and danger are separate so that unauthenticated requests can't
	// fill the queue and lock out signed webhooks. signed is always drained first.
	signed chan updateRequest
	danger chan updateRequest

	// pending tracks updates that are queued but not yet started, so that
	// repeated webhooks for the same image only occupy one slot.
	mu      sync.Mutex
	pending map[queueKey]struct{}
}
