//go:build windows

package pty

import (
	"os/exec"
)

// Start 在 Windows 上通过 ConPTY 启动一个终端会话。
// M1 阶段接入：CreatePseudoConsole + STARTUPINFOEX 的 PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE。
func Start(cmd *exec.Cmd, cfg Config) (Session, error) {
	return nil, ErrNotImplemented
}
