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
	notifier       Notifier
	includeOnly    bool
}

func NewSwarmUpdater(serviceLister ServiceLister, serviceUpdater ServiceUpdater, notifier Notifier, includeOnly bool) *SwarmUpdater {
	return &SwarmUpdater{
		serviceLister:  serviceLister,
		serviceUpdater: serviceUpdater,
		notifier:       notifier,
		includeOnly:    includeOnly,
	}
}

func (u *SwarmUpdater) HandleUpdate(ctx context.Context, image string, tag string) error {
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
		if normalizeTag(extractImageTag(svc.Spec.TaskTemplate.ContainerSpec.Image)) != normalizeTag(tag) {
			continue
		}
		if u.includeOnly {
			if _, included := svc.Spec.Labels[includeLabel]; !included {
				continue
			}
		} else if _, excluded := svc.Spec.Labels[excludeLabel]; excluded {
			continue
		}

		slog.Info("updating swarm service", "service", svc.Spec.Name, "image", image, "tag", tag, "id", svc.ID)
		spec := svc.Spec
		spec.TaskTemplate.ForceUpdate++

		u.notifier.NotifyPending(ctx, image, svc.Spec.Name)
		if _, err := u.serviceUpdater.ServiceUpdate(ctx, svc.ID, svc.Version, spec, swarm.ServiceUpdateOptions{}); err != nil {
			slog.Error("failed to update swarm service", "service", svc.Spec.Name, "image", image, "tag", tag, "id", svc.ID, "error", err)
			u.notifier.NotifyResult(ctx, image, svc.Spec.Name, err)
			errs = append(errs, &ServiceUpdateError{
				ServiceName: svc.Spec.Name,
				ServiceID:   svc.ID,
				Err:         err,
			})
		} else {
			u.notifier.NotifyResult(ctx, image, svc.Spec.Name, nil)
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
