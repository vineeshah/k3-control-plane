//go:build linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	specs "github.com/opencontainers/runtime-spec/specs-go"

	"k8/internal/api"
)

const (
	// containerdNamespace keeps our containers apart from anything else using
	// the same containerd (docker's moby namespace, k3s's k8s.io, ...).
	containerdNamespace = "k8"

	labelAssignment = "k8.assignment"
	labelOwner      = "k8.owner"

	cpuPeriodMicros = 100_000
	stopGracePeriod = 10 * time.Second
)

// ContainerdExecutor runs each assignment as a containerd container whose ID is
// the assignment ID. Containers are owned by containerd, so they keep running
// across agent restarts and are found again by List.
type ContainerdExecutor struct {
	client *containerd.Client
	logDir string

	mu       sync.Mutex
	watching map[string]*watch
}

// watch is this process's view of one container: who to report to, the last
// phase reported (re-sent when Start is called again), and whether an exit is
// expected because we are stopping it.
type watch struct {
	report   Reporter
	phase    api.AssignmentPhase
	message  string
	stopping bool
}

func NewContainerdExecutor(address, logDir string) (*ContainerdExecutor, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	client, err := containerd.New(address, containerd.WithDefaultNamespace(containerdNamespace))
	if err != nil {
		return nil, fmt.Errorf("connect to containerd at %s: %w", address, err)
	}
	return &ContainerdExecutor{
		client:   client,
		logDir:   logDir,
		watching: make(map[string]*watch),
	}, nil
}

func (e *ContainerdExecutor) Close() error {
	return e.client.Close()
}

// Start returns quickly; pulling and creating happen in the background and
// failures are reported as Failed. If the container already exists (agent
// restart) it re-attaches to it instead.
func (e *ContainerdExecutor) Start(_ context.Context, assignment api.Assignment, report Reporter) error {
	e.mu.Lock()
	if w, ok := e.watching[assignment.ID]; ok {
		w.report = report
		phase, message := w.phase, w.message
		e.mu.Unlock()
		if phase != "" {
			report(phase, message)
		}
		return nil
	}
	e.watching[assignment.ID] = &watch{report: report}
	e.mu.Unlock()

	go func() {
		ctx := e.ctx()
		if err := e.run(ctx, assignment); err != nil {
			e.reportFor(assignment.ID, api.AssignmentPhaseFailed, err.Error())
			e.forget(assignment.ID)
		}
	}()
	return nil
}

func (e *ContainerdExecutor) run(ctx context.Context, assignment api.Assignment) error {
	container, err := e.client.LoadContainer(ctx, assignment.ID)
	switch {
	case err == nil:
		return e.attach(ctx, assignment, container)
	case !errdefs.IsNotFound(err):
		return fmt.Errorf("load container: %w", err)
	}

	container, err = e.create(ctx, assignment)
	if err != nil {
		return err
	}
	return e.startTask(ctx, assignment, container)
}

func (e *ContainerdExecutor) create(ctx context.Context, assignment api.Assignment) (containerd.Container, error) {
	image, err := e.ensureImage(ctx, assignment.Image)
	if err != nil {
		return nil, err
	}

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		// No CNI yet: workloads share the node's network, so a service's
		// ports are node ports and the scheduler keeps them from clashing.
		oci.WithHostNamespace(specs.NetworkNamespace),
		oci.WithHostHostsFile,
		oci.WithHostResolvconf,
	}
	if len(assignment.Command) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(assignment.Command...))
	}
	if len(assignment.Env) > 0 {
		specOpts = append(specOpts, oci.WithEnv(envList(assignment.Env)))
	}
	if len(assignment.Volumes) > 0 {
		specOpts = append(specOpts, oci.WithMounts(bindMounts(assignment.Volumes)))
	}
	if assignment.Resources.Memory > 0 {
		specOpts = append(specOpts, oci.WithMemoryLimit(uint64(assignment.Resources.Memory)<<20))
	}
	if assignment.Resources.CPU > 0 {
		// cpu is in millicores: 1000m = one full period of quota.
		quota := int64(assignment.Resources.CPU) * cpuPeriodMicros / 1000
		specOpts = append(specOpts, oci.WithCPUCFS(quota, cpuPeriodMicros))
	}

	container, err := e.client.NewContainer(ctx, assignment.ID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(assignment.ID+"-rootfs", image),
		containerd.WithContainerLabels(map[string]string{
			labelAssignment: assignment.ID,
			labelOwner:      string(assignment.OwnerKind) + "/" + assignment.OwnerName,
		}),
		containerd.WithNewSpec(specOpts...),
	)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	return container, nil
}

func (e *ContainerdExecutor) ensureImage(ctx context.Context, ref string) (containerd.Image, error) {
	named, err := reference.ParseDockerRef(ref)
	if err != nil {
		return nil, fmt.Errorf("parse image %q: %w", ref, err)
	}
	image, err := e.client.GetImage(ctx, named.String())
	if err == nil {
		return image, nil
	}
	if !errdefs.IsNotFound(err) {
		return nil, fmt.Errorf("look up image %s: %w", named, err)
	}
	image, err = e.client.Pull(ctx, named.String(), containerd.WithPullUnpack)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", named, err)
	}
	return image, nil
}

func (e *ContainerdExecutor) startTask(ctx context.Context, assignment api.Assignment, container containerd.Container) error {
	task, err := container.NewTask(ctx, cio.LogFile(e.logPath(assignment.ID)))
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	// Wait must be registered before Start so a fast exit is not missed.
	exitCh, err := task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("wait task: %w", err)
	}
	if err := task.Start(ctx); err != nil {
		_, _ = task.Delete(ctx)
		return fmt.Errorf("start task: %w", err)
	}

	e.reportFor(assignment.ID, api.AssignmentPhaseRunning, "container started")
	go e.watchExit(assignment, exitCh)
	return nil
}

// attach picks up a container that outlived a previous agent process.
func (e *ContainerdExecutor) attach(ctx context.Context, assignment api.Assignment, container containerd.Container) error {
	task, err := container.Task(ctx, nil)
	if errdefs.IsNotFound(err) {
		// The container exists but its process is gone, e.g. after a node
		// reboot. A service is simply started again; a job attempt is
		// reported failed and the controller decides whether to retry it.
		if assignment.OwnerKind == api.WorkloadKindJob {
			return errors.New("job process lost (node restarted?)")
		}
		return e.startTask(ctx, assignment, container)
	}
	if err != nil {
		return fmt.Errorf("load task: %w", err)
	}

	exitCh, err := task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("wait task: %w", err)
	}
	status, err := task.Status(ctx)
	if err != nil {
		return fmt.Errorf("task status: %w", err)
	}
	if status.Status != containerd.Stopped {
		e.reportFor(assignment.ID, api.AssignmentPhaseRunning, "re-attached to running container")
	}
	go e.watchExit(assignment, exitCh)
	return nil
}

func (e *ContainerdExecutor) watchExit(assignment api.Assignment, exitCh <-chan containerd.ExitStatus) {
	exit := <-exitCh

	e.mu.Lock()
	w, ok := e.watching[assignment.ID]
	stopping := ok && w.stopping
	e.mu.Unlock()
	if !ok || stopping {
		// Stop reports for itself.
		return
	}

	code, _, err := exit.Result()
	switch {
	case err != nil:
		e.reportFor(assignment.ID, api.AssignmentPhaseFailed, fmt.Sprintf("wait failed: %v", err))
	case assignment.OwnerKind == api.WorkloadKindJob && code == 0:
		e.reportFor(assignment.ID, api.AssignmentPhaseSucceeded, "exited with code 0")
	case assignment.OwnerKind == api.WorkloadKindJob:
		e.reportFor(assignment.ID, api.AssignmentPhaseFailed, fmt.Sprintf("exited with code %d", code))
	default:
		// A service should never exit on its own. Fail the replica and let
		// the controller schedule a replacement.
		e.reportFor(assignment.ID, api.AssignmentPhaseFailed, fmt.Sprintf("service exited with code %d", code))
	}
	// The exited container is kept until the agent reaps it with Stop, so a
	// restarted agent can still read the result.
}

// Stop sends SIGTERM, escalates to SIGKILL after the grace period, then
// removes the task, the container and its snapshot.
func (e *ContainerdExecutor) Stop(_ context.Context, assignmentID string) error {
	ctx := e.ctx()

	e.mu.Lock()
	w, tracked := e.watching[assignmentID]
	if tracked {
		w.stopping = true
	}
	e.mu.Unlock()
	defer e.forget(assignmentID)

	container, err := e.client.LoadContainer(ctx, assignmentID)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	wasRunning := false
	task, err := container.Task(ctx, nil)
	switch {
	case err == nil:
		wasRunning, err = killTask(ctx, task)
		if err != nil {
			return err
		}
	case !errdefs.IsNotFound(err):
		return fmt.Errorf("load task: %w", err)
	}

	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("delete container: %w", err)
	}
	if wasRunning && tracked {
		w.report(api.AssignmentPhaseStopped, "stopped by agent")
	}
	return nil
}

// killTask stops a task and deletes it. It reports whether the task was still
// running, i.e. whether the stop cut the workload short.
func killTask(ctx context.Context, task containerd.Task) (bool, error) {
	status, err := task.Status(ctx)
	if err != nil {
		return false, fmt.Errorf("task status: %w", err)
	}
	running := status.Status != containerd.Stopped

	if running {
		exitCh, err := task.Wait(ctx)
		if err != nil {
			return false, fmt.Errorf("wait task: %w", err)
		}
		if err := task.Kill(ctx, syscall.SIGTERM); err != nil && !errdefs.IsNotFound(err) {
			return false, fmt.Errorf("sigterm: %w", err)
		}
		select {
		case <-exitCh:
		case <-time.After(stopGracePeriod):
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil && !errdefs.IsNotFound(err) {
				return false, fmt.Errorf("sigkill: %w", err)
			}
			<-exitCh
		}
	}

	if _, err := task.Delete(ctx); err != nil && !errdefs.IsNotFound(err) {
		return false, fmt.Errorf("delete task: %w", err)
	}
	return running, nil
}

func (e *ContainerdExecutor) List(context.Context) ([]string, error) {
	containers, err := e.client.Containers(e.ctx())
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	ids := make([]string, 0, len(containers))
	for _, container := range containers {
		ids = append(ids, container.ID())
	}
	sort.Strings(ids)
	return ids, nil
}

// ctx is detached from any caller: containers and their exit watchers must
// not be torn down when a sync or the agent's context is cancelled.
func (e *ContainerdExecutor) ctx() context.Context {
	return namespaces.WithNamespace(context.Background(), containerdNamespace)
}

func (e *ContainerdExecutor) reportFor(id string, phase api.AssignmentPhase, message string) {
	e.mu.Lock()
	w, ok := e.watching[id]
	if ok {
		w.phase, w.message = phase, message
	}
	e.mu.Unlock()
	if !ok {
		log.Printf("assignment %s: %s (%s), no reporter", id, phase, message)
		return
	}
	w.report(phase, message)
}

func (e *ContainerdExecutor) forget(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.watching, id)
}

func (e *ContainerdExecutor) logPath(id string) string {
	return filepath.Join(e.logDir, id+".log")
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}

func bindMounts(volumes []api.Volume) []specs.Mount {
	mounts := make([]specs.Mount, 0, len(volumes))
	for _, volume := range volumes {
		mounts = append(mounts, specs.Mount{
			Type:        "bind",
			Source:      volume.HostPath,
			Destination: volume.MountPath,
			Options:     []string{"rbind", "rw"},
		})
	}
	return mounts
}
