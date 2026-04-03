package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/docker/docker/api/types/swarm"
)

type SwarmUpdater struct {
	serviceLister  ServiceLister
	serviceUpdater ServiceUpdater
}

func NewSwarmUpdater(serviceLister ServiceLister, serviceUpdater ServiceUpdater) *SwarmUpdater {
	return &SwarmUpdater{
		serviceLister:  serviceLister,
		serviceUpdater: serviceUpdater,
	}
}

func (u *SwarmUpdater) HandleUpdate(ctx context.Context, image string) error {
	services, err := u.serviceLister.ServiceList(ctx, swarm.ServiceListOptions{})
	if err != nil {
		return fmt.Errorf("listing swarm services: %w", err)
	}

	var errs []error
	for _, svc := range services {
		if svc.Spec.TaskTemplate.ContainerSpec == nil {
			continue
		}
		if normalizeImage(svc.Spec.TaskTemplate.ContainerSpec.Image) != normalizeImage(image) {
			continue
		}

		slog.Info("updating swarm service", "service", svc.Spec.Name, "id", svc.ID)
		spec := svc.Spec
		spec.TaskTemplate.ForceUpdate++

		if _, err := u.serviceUpdater.ServiceUpdate(ctx, svc.ID, svc.Version, spec, swarm.ServiceUpdateOptions{}); err != nil {
			slog.Error("failed to update swarm service", "service", svc.Spec.Name, "id", svc.ID, "error", err)
			errs = append(errs, &ServiceUpdateError{
				ServiceName: svc.Spec.Name,
				ServiceID:   svc.ID,
				Err:         err,
			})
		}
	}

	if len(errs) > 0 {
		return &UpdateErrors{Errors: errs}
	}
	return nil
}

type ServiceUpdateError struct {
	ServiceName string
	ServiceID   string
	Err         error
}

func (e *ServiceUpdateError) Error() string {
	return fmt.Sprintf("updating swarm service %s (%s): %s", e.ServiceName, e.ServiceID, e.Err)
}

func (e *ServiceUpdateError) Unwrap() error {
	return e.Err
}
