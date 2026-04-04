package main

import (
	"context"
	"errors"
	"bufio"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/csmith/envflag/v2"
	"github.com/csmith/slogflags"
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

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
	}
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	return info.Main.Version
}

func run() error {
	addr := flag.String("addr", ":8080", "address to listen on")
	secret := flag.String("secret", "", "shared secret(s) for webhook signatures, comma-separated (required)")
	dangerEndpoints := flag.Int("danger-endpoints", 0, "number of unauthenticated webhook endpoints to generate")
	webhookURL := flag.String("webhook-url", "", "URL to send notifications to when updates succeed or fail")
	webhookSecret := flag.String("webhook-secret", "", "secret for signing outgoing notification webhooks")
	includeOnly := flag.Bool("include-only", false, "only update containers/services with the include label")

	envflag.Parse()

	_ = slogflags.Logger(slogflags.WithSetDefault(true))

	slog.Info("starting adze", "version", version())

	secrets := strings.Split(*secret, ",")
	for i := range secrets {
		secrets[i] = strings.TrimSpace(secrets[i])
	}
	secrets = slices.DeleteFunc(secrets, func(s string) bool {
		return s == ""
	})
	if len(secrets) == 0 {
		flag.Usage()
		return fmt.Errorf("secret is required")
	}

	opts := cliflags.NewClientOptions()
	if fi, err := os.Stat(config.Dir()); err == nil && fi.IsDir() {
		opts.ConfigDir = config.Dir()
	}

	dockerCli, err := command.NewDockerCli(
		command.WithOutputStream(newSlogWriter("stdout")),
		command.WithErrorStream(newSlogWriter("stderr")),
	)
	if err != nil {
		return fmt.Errorf("creating docker cli: %w", err)
	}
	if err := dockerCli.Initialize(opts); err != nil {
		return fmt.Errorf("initializing docker cli: %w", err)
	}
	composeService := compose.NewComposeService(dockerCli)

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}

	var notifier Notifier = noopNotifier{}
	if *webhookURL != "" {
		notifier = NewWebhookNotifier(*webhookURL, *webhookSecret)
	}

	updater, err := createUpdater(dockerClient, composeService, notifier, *includeOnly)
	if err != nil {
		return err
	}

	handler := NewHandler(secrets, updater)

	mux := http.NewServeMux()
	mux.Handle("POST /webhook", handler)

	dangerHandler := http.HandlerFunc(handler.ServeHTTPDanger)
	for i := range *dangerEndpoints {
		path := generateEndpointPath(secrets[0], i)
		slog.Info("danger endpoint", "path", "/webhook/"+path)
		mux.Handle("POST /webhook/"+path, dangerHandler)
	}

	srv := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case sig := <-sigCh:
		slog.Info("shutting down", "signal", sig)
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	handler.Shutdown()

	slog.Info("server stopped")
	return nil
}

func createUpdater(dockerClient *client.Client, composeService ComposeUpRunner, notifier Notifier, includeOnly bool) (ImageUpdater, error) {
	info, err := dockerClient.Info(context.Background())
	if err != nil {
		return nil, fmt.Errorf("getting docker info: %w", err)
	}

	if info.Swarm.LocalNodeState == "active" {
		if info.Swarm.ControlAvailable {
			slog.Info("running in swarm mode")
			return NewSwarmUpdater(dockerClient, dockerClient, notifier, includeOnly), nil
		}
		return nil, fmt.Errorf("this node is a swarm worker, adze must run on a swarm manager")
	}

	slog.Info("running in compose mode")
	return NewUpdater(composeService, dockerClient, ComposeProjectLoader{}, notifier, includeOnly), nil
}
