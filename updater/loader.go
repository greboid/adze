package updater

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/v2/cli"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

type ComposeProjectLoader struct{}

func (l ComposeProjectLoader) LoadProject(ctx context.Context, workingDir string, configFiles []string) (*composetypes.Project, error) {
	optFns := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(workingDir),
		cli.WithOsEnv,
		cli.WithConfigFileEnv,
		cli.WithDefaultConfigPath,
	}

	opts, err := cli.NewProjectOptions(configFiles, optFns...)
	if err != nil {
		return nil, fmt.Errorf("creating project options: %w", err)
	}

	project, err := opts.LoadProject(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading project: %w", err)
	}

	return project, nil
}
