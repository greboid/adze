package main

import (
	"context"
	"sync/atomic"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/container"
)

type mockContainerLister struct {
	containers []container.Summary
	err        error
}

func (m *mockContainerLister) ContainerList(_ context.Context, opts container.ListOptions) ([]container.Summary, error) {
	return m.containers, m.err
}

type mockComposeUpRunner struct {
	called  atomic.Bool
	project *composetypes.Project
	options api.UpOptions
	err     error
}

func (m *mockComposeUpRunner) Up(_ context.Context, project *composetypes.Project, options api.UpOptions) error {
	m.called.Store(true)
	m.project = project
	m.options = options
	return m.err
}

type mockProjectLoader struct {
	project     *composetypes.Project
	err         error
	configFiles []string
	workingDir  string
}

func (m *mockProjectLoader) LoadProject(_ context.Context, workingDir string, configFiles []string) (*composetypes.Project, error) {
	m.workingDir = workingDir
	m.configFiles = configFiles
	return m.project, m.err
}
