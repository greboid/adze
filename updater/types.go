package updater

import (
	"context"
	"fmt"
	"strings"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/client"
)

type ContainerLister interface {
	ContainerList(ctx context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error)
}

type ComposeUpRunner interface {
	Up(ctx context.Context, project *composetypes.Project, options api.UpOptions) error
}

type ProjectLoader interface {
	LoadProject(ctx context.Context, workingDir string, configFiles []string) (*composetypes.Project, error)
}

type ServiceLister interface {
	ServiceList(ctx context.Context, options client.ServiceListOptions) (client.ServiceListResult, error)
}

type ServiceUpdater interface {
	ServiceUpdate(ctx context.Context, serviceID string, options client.ServiceUpdateOptions) (client.ServiceUpdateResult, error)
}

type notificationPayload struct {
	Image   string `json:"image"`
	Target  string `json:"target"`
	Dir     string `json:"dir,omitempty"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Private bool   `json:"private"`
}

// Source describes where the image that triggered an update came from.
type Source struct {
	Owner   string
	Private bool
}

type sourceKey struct{}

// WithSource attaches s to ctx for inclusion in outgoing notifications.
func WithSource(ctx context.Context, s Source) context.Context {
	return context.WithValue(ctx, sourceKey{}, s)
}

// sourceFrom treats an unknown source as private so it isn't announced publicly.
func sourceFrom(ctx context.Context) Source {
	if s, ok := ctx.Value(sourceKey{}).(Source); ok {
		return s
	}
	return Source{Private: true}
}

type Notifier interface {
	NotifyPending(ctx context.Context, image string, target string, dir string)
	NotifyResult(ctx context.Context, image string, target string, dir string, err error)
	NotifySkipped(ctx context.Context, image string, target string, dir string, reason string)
}

type NoopNotifier struct{}

func (NoopNotifier) NotifyPending(_ context.Context, _ string, _ string, _ string)           {}
func (NoopNotifier) NotifyResult(_ context.Context, _ string, _ string, _ string, _ error)   {}
func (NoopNotifier) NotifySkipped(_ context.Context, _ string, _ string, _ string, _ string) {}

type ImageUpdater interface {
	HandleUpdate(ctx context.Context, image string, tag string) error
}

type Updater struct {
	composeService ComposeUpRunner
	dockerClient   ContainerLister
	projectLoader  ProjectLoader
	notifier       Notifier
	includeOnly    bool
	lockDirs       bool
	self           *Self
}

type ComposeProject struct {
	WorkingDir   string
	ConfigFiles  string
	ProjectName  string
	OnlySelf     bool
	SelfOutdated bool
}

type ProjectUpdateError struct {
	Project string
	Err     error
}

func (e *ProjectUpdateError) Error() string {
	return fmt.Sprintf("updating project %s: %s", e.Project, e.Err)
}

func (e *ProjectUpdateError) Unwrap() error {
	return e.Err
}

type UpdateErrors struct {
	Errors []error
}

func (e *UpdateErrors) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		msgs[i] = err.Error()
	}
	return "update errors: [" + strings.Join(msgs, ", ") + "]"
}

func (e *UpdateErrors) Unwrap() []error {
	return e.Errors
}

// Option configures an updater.
type Option func(*options)

type options struct {
	lockDirs bool
	self     *Self
}

func newOptions(opts []Option) options {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithDirLocking sets whether a compose project's directory is flocked while it is updated. Defaults to false.
func WithDirLocking(enabled bool) Option {
	return func(o *options) { o.lockDirs = enabled }
}

func WithSelf(self Self) Option {
	return func(o *options) { o.self = &self }
}
