package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/compose-spec/compose-go/v2/cli"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

func validateSignature(secrets []string, headers http.Header, body []byte) bool {
	sig := headers.Get("X-Forgejo-Signature")
	if sig == "" {
		sig = headers.Get("X-Gitea-Signature")
	}
	if sig == "" {
		return false
	}

	sig = strings.TrimPrefix(sig, "sha256=")

	for _, secret := range secrets {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return true
		}
	}

	return false
}

func extractImage(body []byte) string {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return ""
	}
	if p.Package.Type == "container" && p.Package.Owner.Login != "" && p.Package.Name != "" {
		return p.Package.Owner.Login + "/" + p.Package.Name
	}
	if p.Image != "" {
		return p.Image
	}
	return ""
}

func normalizeImage(ref string) string {
	if strings.Contains(ref, "@") {
		ref = strings.SplitN(ref, "@", 2)[0]
	}
	if strings.Contains(ref, ":") {
		ref = strings.SplitN(ref, ":", 2)[0]
	}
	return ref
}

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
