package updater

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/client"
)

// newSlogWriter returns an io.Writer that logs each line via slog.Debug in a background goroutine.
func newSlogWriter(stream string) io.Writer {
	r, w := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if line := scanner.Text(); line != "" {
				slog.Debug(line, "stream", stream)
			}
		}
	}()
	return w
}

// NewFromEnv connects to docker using the standard environment variables and docker config,
// and returns a compose or swarm updater depending on how the daemon is configured.
func NewFromEnv(ctx context.Context, notifier Notifier, includeOnly bool) (ImageUpdater, error) {
	opts := cliflags.NewClientOptions()
	if fi, err := os.Stat(config.Dir()); err == nil && fi.IsDir() {
		opts.ConfigDir = config.Dir()
	}

	dockerCli, err := command.NewDockerCli(
		command.WithOutputStream(newSlogWriter("stdout")),
		command.WithErrorStream(newSlogWriter("stderr")),
	)
	if err != nil {
		return nil, fmt.Errorf("creating docker cli: %w", err)
	}
	if err := dockerCli.Initialize(opts); err != nil {
		return nil, fmt.Errorf("initializing docker cli: %w", err)
	}
	composeService := compose.NewComposeService(dockerCli)

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return New(ctx, dockerClient, composeService, notifier, includeOnly)
}

// New returns a swarm updater if the docker daemon is a swarm manager, or a compose updater otherwise.
func New(ctx context.Context, dockerClient *client.Client, composeService ComposeUpRunner, notifier Notifier, includeOnly bool) (ImageUpdater, error) {
	info, err := dockerClient.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting docker info: %w", err)
	}

	mode := "exclude"
	if includeOnly {
		mode = "include"
	}

	if info.Swarm.LocalNodeState == "active" {
		if info.Swarm.ControlAvailable {
			slog.Info("configuration", "docker", "swarm", "selection", mode)
			return NewSwarmUpdater(dockerClient, dockerClient, notifier, includeOnly), nil
		}
		return nil, fmt.Errorf("this node is a swarm worker, adze must run on a swarm manager")
	}

	slog.Info("configuration", "docker", "compose", "selection", mode)
	return NewUpdater(composeService, dockerClient, ComposeProjectLoader{}, notifier, includeOnly), nil
}
