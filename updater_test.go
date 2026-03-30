package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/container"
)

func TestFindComposeProjects_MatchingContainer(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image:  "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:    "myproject",
					api.WorkingDirLabel: "/opt/myapp",
					api.ConfigFilesLabel: "/opt/myapp/docker-compose.yml",
				},
			},
		},
	}

	u := NewUpdater(nil, lister, nil)
	projects, err := u.findComposeProjects(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ProjectName != "myproject" {
		t.Errorf("expected project name %q, got %q", "myproject", projects[0].ProjectName)
	}
	if projects[0].WorkingDir != "/opt/myapp" {
		t.Errorf("expected working dir %q, got %q", "/opt/myapp", projects[0].WorkingDir)
	}
}

func TestFindComposeProjects_NoMatch(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "otherapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:    "other",
					api.WorkingDirLabel: "/opt/other",
				},
			},
		},
	}

	u := NewUpdater(nil, lister, nil)
	projects, err := u.findComposeProjects(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

func TestFindComposeProjects_Deduplicates(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:    "myproject",
					api.WorkingDirLabel: "/opt/myapp",
				},
			},
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:    "myproject",
					api.WorkingDirLabel: "/opt/myapp",
				},
			},
		},
	}

	u := NewUpdater(nil, lister, nil)
	projects, err := u.findComposeProjects(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project (deduplicated), got %d", len(projects))
	}
}

func TestFindComposeProjects_SkipsNonCompose(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image:  "myapp:latest",
				Labels: map[string]string{},
			},
		},
	}

	u := NewUpdater(nil, lister, nil)
	projects, err := u.findComposeProjects(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(projects) != 0 {
		t.Fatalf("expected 0 projects for non-compose container, got %d", len(projects))
	}
}

func TestFindComposeProjects_ListError(t *testing.T) {
	lister := &mockContainerLister{
		err: fmt.Errorf("docker error"),
	}

	u := NewUpdater(nil, lister, nil)
	_, err := u.findComposeProjects(context.Background(), "myapp:latest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestHandleUpdate_NoProjects(t *testing.T) {
	lister := &mockContainerLister{containers: nil}
	up := &mockComposeUpRunner{}
	u := NewUpdater(up, lister, nil)

	err := u.HandleUpdate(context.Background(), "nonexistent:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if up.called.Load() {
		t.Error("expected Up not to be called when no projects found")
	}
}

func TestHandleUpdate_WithProject(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:     "myproject",
					api.WorkingDirLabel:  "/opt/myapp",
					api.ConfigFilesLabel: "/opt/myapp/docker-compose.yml",
				},
			},
		},
	}

	loader := &mockProjectLoader{
		project: &composetypes.Project{
			Services: composetypes.Services{
				"web": {Name: "web"},
			},
		},
	}
	up := &mockComposeUpRunner{}
	u := NewUpdater(up, lister, loader)

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !up.called.Load() {
		t.Error("expected Up to be called")
	}
	if up.project.Services["web"].PullPolicy != "always" {
		t.Error("expected PullPolicy to be set to 'always'")
	}
}

func TestHandleUpdate_ComposeUpError(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:    "myproject",
					api.WorkingDirLabel: "/opt/myapp",
				},
			},
		},
	}

	loader := &mockProjectLoader{
		project: &composetypes.Project{
			Services: composetypes.Services{
				"web": {Name: "web"},
			},
		},
	}
	upErr := fmt.Errorf("compose up failed")
	up := &mockComposeUpRunner{err: upErr}
	u := NewUpdater(up, lister, loader)

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
	var projErr *ProjectUpdateError
	if !errors.As(updateErrors.Errors[0], &projErr) {
		t.Fatalf("expected ProjectUpdateError, got: %T: %v", updateErrors.Errors[0], updateErrors.Errors[0])
	}
	if projErr.Project != "myproject" {
		t.Errorf("expected project myproject, got %s", projErr.Project)
	}
	if !errors.Is(err, upErr) {
		t.Errorf("expected error chain to contain upErr, got: %v", err)
	}
}

func TestHandleUpdate_ProjectLoadError(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:    "myproject",
					api.WorkingDirLabel: "/opt/myapp",
				},
			},
		},
	}

	loader := &mockProjectLoader{err: fmt.Errorf("load failed")}
	up := &mockComposeUpRunner{}
	u := NewUpdater(up, lister, loader)

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestHandleUpdate_MultipleConfigFiles(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:     "myproject",
					api.WorkingDirLabel:  "/opt/myapp",
					api.ConfigFilesLabel: "/opt/myapp/docker-compose.yml,/opt/myapp/docker-compose.override.yml",
				},
			},
		},
	}

	loader := &mockProjectLoader{
		project: &composetypes.Project{
			Services: composetypes.Services{
				"web": {Name: "web"},
			},
		},
	}
	up := &mockComposeUpRunner{}
	u := NewUpdater(up, lister, loader)

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !up.called.Load() {
		t.Fatal("expected Up to be called")
	}

	expectedFiles := []string{"/opt/myapp/docker-compose.yml", "/opt/myapp/docker-compose.override.yml"}
	if len(loader.configFiles) != len(expectedFiles) {
		t.Fatalf("expected %d config files, got %d: %v", len(expectedFiles), len(loader.configFiles), loader.configFiles)
	}
	for i, expected := range expectedFiles {
		if loader.configFiles[i] != expected {
			t.Errorf("configFiles[%d] = %q, want %q", i, loader.configFiles[i], expected)
		}
	}
}

func TestHandleUpdate_SingleConfigFile(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:     "myproject",
					api.WorkingDirLabel:  "/opt/myapp",
					api.ConfigFilesLabel: "/opt/myapp/docker-compose.yml",
				},
			},
		},
	}

	loader := &mockProjectLoader{
		project: &composetypes.Project{
			Services: composetypes.Services{
				"web": {Name: "web"},
			},
		},
	}
	up := &mockComposeUpRunner{}
	u := NewUpdater(up, lister, loader)

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(loader.configFiles) != 1 || loader.configFiles[0] != "/opt/myapp/docker-compose.yml" {
		t.Errorf("expected 1 config file, got %v", loader.configFiles)
	}
}

func TestHandleUpdate_NoConfigFiles(t *testing.T) {
	lister := &mockContainerLister{
		containers: []container.Summary{
			{
				Image: "myapp:latest",
				Labels: map[string]string{
					api.ProjectLabel:    "myproject",
					api.WorkingDirLabel: "/opt/myapp",
				},
			},
		},
	}

	loader := &mockProjectLoader{
		project: &composetypes.Project{
			Services: composetypes.Services{
				"web": {Name: "web"},
			},
		},
	}
	up := &mockComposeUpRunner{}
	u := NewUpdater(up, lister, loader)

	err := u.HandleUpdate(context.Background(), "myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(loader.configFiles) != 0 {
		t.Errorf("expected no config files, got %v", loader.configFiles)
	}
}

