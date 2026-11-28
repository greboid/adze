package updater

import (
	"context"
	"errors"
	"strings"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type mockContainerInspector struct {
	containers map[string]container.InspectResponse
}

func (m *mockContainerInspector) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	c, ok := m.containers[id]
	if !ok {
		return client.ContainerInspectResult{}, errors.New("no such container")
	}
	return client.ContainerInspectResult{Container: c}, nil
}

type recordingNotifier struct {
	events []string
}

func (r *recordingNotifier) NotifyPending(_ context.Context, _, target, _ string) {
	r.events = append(r.events, "pending:"+target)
}

func (r *recordingNotifier) NotifyResult(_ context.Context, _, target, _ string, err error) {
	if err != nil {
		r.events = append(r.events, "failure:"+target)
		return
	}
	r.events = append(r.events, "success:"+target)
}

func (r *recordingNotifier) NotifySkipped(_ context.Context, _, target, _, _ string) {
	r.events = append(r.events, "skipped:"+target)
}

const testContainerID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestContainerIDFromMountinfo(t *testing.T) {
	mountinfo := `1 0 0:1 / / rw - overlay overlay rw
2 1 254:1 /var/lib/docker/containers/` + testContainerID + `/hostname /etc/hostname rw - ext4 /dev/vda1 rw
`
	if got := containerIDFromMountinfo(strings.NewReader(mountinfo)); got != testContainerID {
		t.Errorf("expected %q, got %q", testContainerID, got)
	}
	if got := containerIDFromMountinfo(strings.NewReader("1 0 0:1 / / rw - ext4 /dev/sda1 rw\n")); got != "" {
		t.Errorf("expected no id, got %q", got)
	}
}

func TestDetectSelf(t *testing.T) {
	inspector := &mockContainerInspector{containers: map[string]container.InspectResponse{
		"abc123": {Config: &container.Config{
			Image:  "adze:latest",
			Labels: map[string]string{api.ProjectLabel: "infra", api.ServiceLabel: "adze"},
		}},
	}}

	self, ok := detectSelf(context.Background(), inspector, []string{"missing", "abc123"})
	if !ok {
		t.Fatal("expected self to be detected")
	}
	if self != (Self{Project: "infra", Service: "adze"}) {
		t.Errorf("unexpected self %+v", self)
	}
}

func TestDetectSelf_NotCompose(t *testing.T) {
	inspector := &mockContainerInspector{containers: map[string]container.InspectResponse{
		"abc123": {Config: &container.Config{Image: "adze:latest"}},
	}}
	if _, ok := detectSelf(context.Background(), inspector, []string{"abc123"}); ok {
		t.Error("expected no self for container without compose labels")
	}
}

func selfTestLister() *mockContainerLister {
	return &mockContainerLister{containers: []container.Summary{
		{
			Image:  "adze:latest",
			Labels: map[string]string{api.ProjectLabel: "infra", api.ServiceLabel: "adze", api.WorkingDirLabel: "/opt/infra"},
		},
		{
			Image:  "web:latest",
			Labels: map[string]string{api.ProjectLabel: "infra", api.ServiceLabel: "web", api.WorkingDirLabel: "/opt/infra"},
		},
	}}
}

func selfTestLoader() *mockProjectLoader {
	return &mockProjectLoader{project: &composetypes.Project{
		Name: "infra",
		Services: composetypes.Services{
			"adze": {Name: "adze"},
			"web":  {Name: "web", Image: "web:latest"},
		},
	}}
}

func TestHandleUpdate_SelfImageSkipped(t *testing.T) {
	up := &mockComposeUpRunner{}
	n := &recordingNotifier{}
	u := NewUpdater(up, selfTestLister(), selfTestLoader(), n, false, WithSelf(Self{Project: "infra", Service: "adze"}))

	if err := u.HandleUpdate(context.Background(), "adze", "latest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.called.Load() {
		t.Error("expected Up not to be called when only adze matched")
	}
	if strings.Join(n.events, ",") != "skipped:infra" {
		t.Errorf("unexpected notifications %v", n.events)
	}
}

func TestHandleUpdate_SelfProjectOtherImage(t *testing.T) {
	up := &mockComposeUpRunner{}
	n := &recordingNotifier{}
	u := NewUpdater(up, selfTestLister(), selfTestLoader(), n, false, WithSelf(Self{Project: "infra", Service: "adze"}))

	if err := u.HandleUpdate(context.Background(), "web", "latest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up.called.Load() {
		t.Fatal("expected Up to be called")
	}
	if _, ok := up.project.Services["adze"]; ok {
		t.Error("expected adze service to be excluded from up")
	}
	if _, ok := up.project.DisabledServices["adze"]; !ok {
		t.Error("expected adze service to be disabled")
	}
	if _, ok := up.project.Services["web"]; !ok {
		t.Error("expected web service to be updated")
	}
	if strings.Join(n.events, ",") != "pending:infra,success:infra" {
		t.Errorf("unexpected notifications %v", n.events)
	}
}

func TestHandleUpdate_SelfAndOtherSameImage(t *testing.T) {
	lister := selfTestLister()
	lister.containers[1].Image = "adze:latest"
	up := &mockComposeUpRunner{}
	n := &recordingNotifier{}
	u := NewUpdater(up, lister, selfTestLoader(), n, false, WithSelf(Self{Project: "infra", Service: "adze"}))

	if err := u.HandleUpdate(context.Background(), "adze", "latest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !up.called.Load() {
		t.Fatal("expected Up to be called for the other service")
	}
	if strings.Join(n.events, ",") != "pending:infra,success:infra,skipped:infra" {
		t.Errorf("unexpected notifications %v", n.events)
	}
}
