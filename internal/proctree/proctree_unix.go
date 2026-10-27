//go:build !windows

package proctree

import (
	"os/exec"
	"syscall"
)

// New 创建 Unix 进程组。
//
// 待接入：启动时给 cmd.SysProcAttr 设 Setpgid: true，终止时对 -pgid 发
// SIGTERM，等一小段时间还没退再补 SIGKILL。
//
// 之所以现在还是空壳：M0/M1 只面向 Windows，但接口已经按最终形态定好了，
// 补 mac/Linux 时只加这个文件，业务层不用动。
func New() (Group, error) {
	return nil, ErrNotImplemented
}

type unixGroup struct {
	cmd *exec.Cmd
	pgid int
}

func (g *unixGroup) Handle() uintptr { return 0 }

func (g *unixGroup) Kill() error {
	if g.pgid <= 0 {
		return nil
	}
	// 负号表示对整组发信号。
	return syscall.Kill(-g.pgid, syscall.SIGKILL)
}

func (g *unixGroup) Usage() (Usage, error) {
	return Usage{}, ErrNotImplemented
}

func (g *unixGroup) Close() error { return nil }
