package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/swarm"
)

func makeService(id, name, image string, versionIndex uint64, forceUpdate uint64) swarm.Service {
	return makeServiceWithLabels(id, name, image, versionIndex, forceUpdate, nil)
}

func makeServiceWithLabels(id, name, image string, versionIndex uint64, forceUpdate uint64, labels map[string]string) swarm.Service {
	return swarm.Service{
		ID: id,
		Meta: swarm.Meta{
			Version: swarm.Version{Index: versionIndex},
		},
		Spec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name:   name,
				Labels: labels,
			},
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{
					Image: image,
				},
				ForceUpdate: forceUpdate,
			},
		},
	}
}

func TestSwarmHandleUpdate_NoMatch(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeService("svc1", "other_web", "otherapp:latest", 1, 0),
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.called.Load() {
		t.Error("expected ServiceUpdate not to be called")
	}
}

func TestSwarmHandleUpdate_MatchingService(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeService("svc123", "myapp_web", "myapp:latest", 42, 0),
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up.called.Load() {
		t.Fatal("expected ServiceUpdate to be called")
	}
	if up.serviceID != "svc123" {
		t.Errorf("expected serviceID svc123, got %s", up.serviceID)
	}
	if up.version.Index != 42 {
		t.Errorf("expected version index 42, got %d", up.version.Index)
	}
	if up.spec.TaskTemplate.ForceUpdate != 1 {
		t.Errorf("expected ForceUpdate=1, got %d", up.spec.TaskTemplate.ForceUpdate)
	}
}

func TestSwarmHandleUpdate_MultipleMatchingServices(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeService("svc1", "mystack_web", "myapp:latest", 10, 0),
			makeService("svc2", "mystack_worker", "myapp:latest", 11, 0),
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !up.called.Load() {
		t.Fatal("expected ServiceUpdate to be called")
	}
	if up.serviceID != "svc2" {
		t.Errorf("expected last serviceID svc2, got %s", up.serviceID)
	}
}

func TestSwarmHandleUpdate_UpdateError(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeService("svc1", "myapp_web", "myapp:latest", 1, 0),
		},
	}
	upErr := fmt.Errorf("update failed")
	up := &mockServiceUpdater{err: upErr}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var updateErrors *UpdateErrors
	if !errors.As(err, &updateErrors) {
		t.Fatalf("expected UpdateErrors, got: %T: %v", err, err)
	}
	if len(updateErrors.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(updateErrors.Errors))
	}
	var svcErr *ServiceUpdateError
	if !errors.As(updateErrors.Errors[0], &svcErr) {
		t.Fatalf("expected ServiceUpdateError, got: %T", updateErrors.Errors[0])
	}
	if svcErr.ServiceName != "myapp_web" {
		t.Errorf("expected service name myapp_web, got %s", svcErr.ServiceName)
	}
	if svcErr.ServiceID != "svc1" {
		t.Errorf("expected service ID svc1, got %s", svcErr.ServiceID)
	}
	if !errors.Is(err, upErr) {
		t.Errorf("expected error chain to contain upErr")
	}
}

func TestSwarmHandleUpdate_NilContainerSpec(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			{
				ID: "svc1",
				Spec: swarm.ServiceSpec{
					Annotations: swarm.Annotations{Name: "myapp_web"},
					TaskTemplate: swarm.TaskSpec{
						ContainerSpec: nil,
					},
				},
			},
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.called.Load() {
		t.Error("expected ServiceUpdate not to be called for nil ContainerSpec")
	}
}

func TestSwarmHandleUpdate_ServiceListError(t *testing.T) {
	lister := &mockServiceLister{err: fmt.Errorf("docker error")}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSwarmHandleUpdate_ForceUpdateIncrements(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeService("svc1", "myapp_web", "myapp:latest", 1, 5),
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.spec.TaskTemplate.ForceUpdate != 6 {
		t.Errorf("expected ForceUpdate=6, got %d", up.spec.TaskTemplate.ForceUpdate)
	}
}

func TestSwarmHandleUpdate_ImageNormalization(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeService("svc1", "myapp_web", "myapp:latest", 1, 0),
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:v2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up.called.Load() {
		t.Error("expected ServiceUpdate to be called for matching base image")
	}
}

func TestSwarmHandleUpdate_ExcludedService(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeServiceWithLabels("svc1", "myapp_web", "myapp:latest", 1, 0, map[string]string{
				excludeLabel: "true",
			}),
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.called.Load() {
		t.Error("expected ServiceUpdate not to be called for excluded service")
	}
}

func TestSwarmHandleUpdate_ExcludedDoesNotAffectOthers(t *testing.T) {
	lister := &mockServiceLister{
		services: []swarm.Service{
			makeServiceWithLabels("svc1", "excluded_web", "myapp:latest", 1, 0, map[string]string{
				excludeLabel: "true",
			}),
			makeService("svc2", "included_web", "myapp:latest", 2, 0),
		},
	}
	up := &mockServiceUpdater{}
	u := NewSwarmUpdater(lister, up, noopNotifier{})

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up.called.Load() {
		t.Fatal("expected ServiceUpdate to be called for non-excluded service")
	}
	if up.serviceID != "svc2" {
		t.Errorf("expected serviceID svc2, got %s", up.serviceID)
	}
}
