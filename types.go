package main

import (
	"context"
	"fmt"
	"strings"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/swarm"
)

type ContainerLister interface {
	ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error)
}

type ComposeUpRunner interface {
	Up(ctx context.Context, project *composetypes.Project, options api.UpOptions) error
}

type ProjectLoader interface {
	LoadProject(ctx context.Context, workingDir string, configFiles []string) (*composetypes.Project, error)
}

type ServiceLister interface {
	ServiceList(ctx context.Context, options swarm.ServiceListOptions) ([]swarm.Service, error)
}

type ServiceUpdater interface {
	ServiceUpdate(ctx context.Context, serviceID string, version swarm.Version, spec swarm.ServiceSpec, options swarm.ServiceUpdateOptions) (swarm.ServiceUpdateResponse, error)
}

type ImageUpdater interface {
	HandleUpdate(ctx context.Context, image string) error
}

type Updater struct {
	composeService ComposeUpRunner
	dockerClient   ContainerLister
	projectLoader  ProjectLoader
}

type ComposeProject struct {
	WorkingDir  string
	ConfigFiles string
	ProjectName string
}

type ProjectUpdateError struct {
	Project string
	Err     error
}

func (e *ProjectUpdateError) Error() string {
	return fmt.Sprintf("updating project %s: %s", e.Project, e.Err)
}

func (e *ProjectUpdateError) Unwrap() error {
	return e.Err
}

type UpdateErrors struct {
	Errors []error
}

func (e *UpdateErrors) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		msgs[i] = err.Error()
	}
	return "update errors: [" + strings.Join(msgs, ", ") + "]"
}

func (e *UpdateErrors) Unwrap() []error {
	return e.Errors
}

type webhookPayload struct {
	Image   string `json:"image"`
	Package struct {
		Owner struct {
			Login   string `json:"login"`
			HTMLURL string `json:"html_url"`
		} `json:"owner"`
		Type        string `json:"type"`
		PackageType string `json:"package_type"`
		Name        string `json:"name"`
	} `json:"package"`
	Events []struct {
		Action string `json:"action"`
		Target struct {
			Repository string `json:"repository"`
			MediaType  string `json:"mediaType"`
		} `json:"target"`
		Request struct {
			Host string `json:"host"`
		} `json:"request"`
	} `json:"events"`
}

type updateRequest struct {
	ctx   context.Context
	image string
}

type Handler struct {
	secrets []string
	updater ImageUpdater
	updates chan updateRequest
}
