package updater

import (
	"context"
	"sync/atomic"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

type mockContainerLister struct {
	containers []container.Summary
	err        error
}

func (m *mockContainerLister) ContainerList(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{Items: m.containers}, m.err
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

type mockServiceLister struct {
	services []swarm.Service
	err      error
}

func (m *mockServiceLister) ServiceList(_ context.Context, _ client.ServiceListOptions) (client.ServiceListResult, error) {
	return client.ServiceListResult{Items: m.services}, m.err
}

type mockServiceUpdater struct {
	called    atomic.Bool
	serviceID string
	version   swarm.Version
	spec      swarm.ServiceSpec
	err       error
}

func (m *mockServiceUpdater) ServiceUpdate(_ context.Context, serviceID string, options client.ServiceUpdateOptions) (client.ServiceUpdateResult, error) {
	m.called.Store(true)
	m.serviceID = serviceID
	m.version = options.Version
	m.spec = options.Spec
	return client.ServiceUpdateResult{}, m.err
}
