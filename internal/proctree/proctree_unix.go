//go:build !windows

package proctree

import (
	"os/exec"
)

// New 创建 Unix 进程组。
// M1 阶段接入：SysProcAttr{Setpgid: true}，终止时对 -pgid 发信号。
func New() (Group, error) {
	return nil, ErrNotImplemented
}

type unixGroup struct {
	cmd *exec.Cmd
}

func (g *unixGroup) Attach(cmd *exec.Cmd) error { return ErrNotImplemented }
func (g *unixGroup) Kill() error               { return ErrNotImplemented }
