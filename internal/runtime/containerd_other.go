//go:build !linux

package runtime

import (
	"context"
	"errors"

	"k8/internal/api"
)

// ContainerdExecutor is only available on Linux, where containerd runs.
type ContainerdExecutor struct{}

func NewContainerdExecutor(address, logDir string) (*ContainerdExecutor, error) {
	return nil, errors.New("containerd runtime requires linux; use -runtime sim")
}

func (e *ContainerdExecutor) Close() error { return nil }

func (e *ContainerdExecutor) Start(context.Context, api.Assignment, Reporter) error {
	return errors.New("containerd runtime requires linux")
}

func (e *ContainerdExecutor) Stop(context.Context, string) error {
	return errors.New("containerd runtime requires linux")
}

func (e *ContainerdExecutor) List(context.Context) ([]string, error) {
	return nil, errors.New("containerd runtime requires linux")
}
