package updater

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/client"
)

type Self struct {
	Project string
	Service string
}

type ContainerInspector interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

var mountinfoIDPattern = regexp.MustCompile(`containers/([0-9a-f]{64})/`)

func containerIDFromMountinfo(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if m := mountinfoIDPattern.FindStringSubmatch(scanner.Text()); m != nil {
			return m[1]
		}
	}
	return ""
}

func containerIDCandidates() []string {
	var ids []string
	if f, err := os.Open("/proc/self/mountinfo"); err == nil {
		if id := containerIDFromMountinfo(f); id != "" {
			ids = append(ids, id)
		}
		f.Close()
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		ids = append(ids, h)
	}
	return ids
}

func DetectSelf(ctx context.Context, inspector ContainerInspector) (Self, bool) {
	return detectSelf(ctx, inspector, containerIDCandidates())
}

func detectSelf(ctx context.Context, inspector ContainerInspector, ids []string) (Self, bool) {
	for _, id := range ids {
		res, err := inspector.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		if err != nil {
			slog.Debug("not adze's container", "id", id, "error", err)
			continue
		}
		if res.Container.Config == nil {
			continue
		}
		labels := res.Container.Config.Labels
		self := Self{
			Project: labels[api.ProjectLabel],
			Service: labels[api.ServiceLabel],
		}
		if self.Project == "" || self.Service == "" {
			return Self{}, false
		}
		return self, true
	}
	return Self{}, false
}
