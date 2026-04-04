package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

const excludeLabel = "com.greboid.adze.excluded"
const includeLabel = "com.greboid.adze.included"

func NewUpdater(composeService ComposeUpRunner, dockerClient ContainerLister, projectLoader ProjectLoader, notifier Notifier, includeOnly bool) *Updater {
	return &Updater{
		composeService: composeService,
		dockerClient:   dockerClient,
		projectLoader:  projectLoader,
		notifier:       notifier,
		includeOnly:    includeOnly,
	}
}

func (u *Updater) HandleUpdate(ctx context.Context, image string, tag string) error {
	projects, err := u.findComposeProjects(ctx, image, tag)
	if err != nil {
		return fmt.Errorf("finding compose projects: %w", err)
	}

	if len(projects) == 0 {
		slog.Info("no compose projects found", "image", image, "tag", tag)
		return nil
	}

	var errs []error
	for _, proj := range projects {
		slog.Info("updating compose project", "project", proj.ProjectName, "dir", proj.WorkingDir, "config", proj.ConfigFiles)
		u.notifier.NotifyPending(ctx, image, proj.ProjectName)
		if err := u.updateProject(ctx, proj); err != nil {
			slog.Error("failed to update project", "project", proj.ProjectName, "error", err)
			u.notifier.NotifyResult(ctx, image, proj.ProjectName, err)
			errs = append(errs, &ProjectUpdateError{Project: proj.ProjectName, Err: err})
		} else {
			u.notifier.NotifyResult(ctx, image, proj.ProjectName, nil)
		}
	}
	if len(errs) > 0 {
		return &UpdateErrors{Errors: errs}
	}
	return nil
}

func (u *Updater) runUp(ctx context.Context, workingDir string, configFiles []string) error {
	project, err := u.projectLoader.LoadProject(ctx, workingDir, configFiles)
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	for name, svc := range project.Services {
		svc.PullPolicy = "always"
		svc.CustomLabels = composetypes.Labels{
			api.ProjectLabel:     project.Name,
			api.ServiceLabel:     name,
			api.VersionLabel:     api.ComposeVersion,
			api.WorkingDirLabel:  project.WorkingDir,
			api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
			api.OneoffLabel:      "False",
		}
		project.Services[name] = svc
	}

	err = u.composeService.Up(ctx, project, api.UpOptions{
		Create: api.CreateOptions{
			RemoveOrphans: true,
		},
		Start: api.StartOptions{
			Project: project,
			Wait:    true,
		},
	})
	if err != nil {
		return fmt.Errorf("running compose up: %w", err)
	}

	return nil
}

func (u *Updater) findComposeProjects(ctx context.Context, image string, tag string) ([]ComposeProject, error) {
	containers, err := u.dockerClient.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "status",
			Value: "running",
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	seen := make(map[string]bool)
	var projects []ComposeProject

	for _, c := range containers {
		if normalizeImage(c.Image) != normalizeImage(image) {
			continue
		}
		if normalizeTag(extractImageTag(c.Image)) != normalizeTag(tag) {
			continue
		}
		if u.includeOnly {
			if _, included := c.Labels[includeLabel]; !included {
				continue
			}
		} else if _, excluded := c.Labels[excludeLabel]; excluded {
			continue
		}
		workingDir := c.Labels[api.WorkingDirLabel]
		configFiles := c.Labels[api.ConfigFilesLabel]
		projectName := c.Labels[api.ProjectLabel]

		if workingDir == "" || projectName == "" {
			continue
		}

		if seen[projectName] {
			continue
		}
		seen[projectName] = true

		projects = append(projects, ComposeProject{
			WorkingDir:  workingDir,
			ConfigFiles: configFiles,
			ProjectName: projectName,
		})
	}

	return projects, nil
}

func (u *Updater) updateProject(ctx context.Context, proj ComposeProject) error {
	var files []string
	if proj.ConfigFiles != "" {
		files = strings.Split(proj.ConfigFiles, ",")
	}

	slog.Info("updating services")
	if err := u.runUp(ctx, proj.WorkingDir, files); err != nil {
		return fmt.Errorf("up failed: %w", err)
	}

	return nil
}
