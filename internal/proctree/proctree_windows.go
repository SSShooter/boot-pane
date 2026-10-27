//go:build windows

package proctree

import (
	"os/exec"
)

// New 创建 Windows 进程组。
// M1 阶段接入：CreateJobObject + JOBOBJECT_EXTENDED_LIMIT_INFORMATION
// 的 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE，并用 CREATE_SUSPENDED 保证子进程
// 在加入 Job 之前不会先派生出孙进程。
func New() (Group, error) {
	return nil, ErrNotImplemented
}

type windowsGroup struct {
	cmd *exec.Cmd
}

func (g *windowsGroup) Attach(cmd *exec.Cmd) error { return ErrNotImplemented }
func (g *windowsGroup) Kill() error               { return ErrNotImplemented }
