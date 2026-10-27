// Package proctree 负责把进程及其所有子孙进程一起管理、一起终止。
//
// 为什么必须有这一层：`pnpm dev` 实际会派生出 pnpm → node → vite 三层进程。
// 只 kill 最外层的 pnpm，node 会变成孤儿继续占着 5173 端口，用户下次启动就会
// 撞上 EADDRINUSE，而且翻遍任务管理器也找不到是谁占的。
//
// Windows 用 Job Object（KILL_ON_JOB_CLOSE），Unix 用 Setpgid + kill(-pgid)。
// 平台差异收敛在 proctree_windows.go / proctree_unix.go，业务层只依赖 Group 接口。
package proctree

import (
	"errors"
	"os/exec"
)

// ErrNotImplemented 表示当前平台实现尚未接入。
var ErrNotImplemented = errors.New("进程树管理尚未实现")

// Group 是一组需要同生共死的进程。
type Group interface {
	// Attach 把即将启动的进程纳入管理。必须在 cmd.Start 之前或紧随其后调用，
	// 具体时机由平台实现决定（Windows 需要进程处于挂起状态）。
	Attach(cmd *exec.Cmd) error
	// Kill 终止整棵进程树，包括已经脱离父进程的孙子进程。
	Kill() error
}
