package main

import (
	"context"

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
	ctx   context.Context
	image string
	tag   string
}

type Handler struct {
	secrets []string
	updater updater.ImageUpdater
	updates chan updateRequest
}
