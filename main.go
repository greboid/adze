package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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
	"github.com/greboid/adze/updater"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
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
	printDangerEndpoints := flag.Bool("print-danger-endpoints", false, "print the unauthenticated webhook endpoint paths and exit")

	envflag.Parse()

	_ = slogflags.Logger(slogflags.WithSetDefault(true))

	slog.Info("starting adze", "version", version(), "addr", *addr)

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

	if *printDangerEndpoints {
		for i := range *dangerEndpoints {
			fmt.Println("/webhook/" + generateEndpointPath(secrets[0], i))
		}
		return nil
	}

	var notifier updater.Notifier = updater.NoopNotifier{}
	if *webhookURL != "" {
		slog.Info("notifications enabled", "url", updater.RedactURL(*webhookURL))
		notifier = updater.NewWebhookNotifier(*webhookURL, *webhookSecret)
	}

	imageUpdater, err := updater.NewFromEnv(context.Background(), notifier, *includeOnly)
	if err != nil {
		return err
	}

	handler := NewHandler(secrets, imageUpdater)

	mux := http.NewServeMux()
	mux.Handle("POST /webhook", handler)

	dangerHandler := http.HandlerFunc(handler.ServeHTTPDanger)
	for i := range *dangerEndpoints {
		mux.Handle("POST /webhook/"+generateEndpointPath(secrets[0], i), dangerHandler)
	}
	if *dangerEndpoints > 0 {
		slog.Warn("unauthenticated webhook endpoints enabled, use -print-danger-endpoints to list them", "count", *dangerEndpoints)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
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
		slog.Debug("shutdown context expired", "error", err)
		return fmt.Errorf("shutdown: %w", err)
	}

	handler.Shutdown()

	slog.Info("server stopped")
	return nil
}
