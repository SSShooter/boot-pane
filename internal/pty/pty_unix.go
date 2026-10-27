//go:build !windows

package pty

import (
	"os/exec"
)

// Start 在 Unix 上通过 creack/pty 启动一个终端会话。
// M1 阶段接入。
func Start(cmd *exec.Cmd, cfg Config) (Session, error) {
	return nil, ErrNotImplemented
}
