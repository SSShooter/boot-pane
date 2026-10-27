// Package pty 提供跨平台的伪终端（Windows 走 ConPTY，Unix 走 pty）。
//
// 为什么不能直接用 cmd.StdoutPipe：vite / webpack / uni-app 这类工具会检测
// 输出是不是 TTY，非 TTY 时会关掉颜色、禁用 spinner 重绘，日志会退化成一堆
// 冗余的重复行。只有接上真正的伪终端，才能拿到和终端里一模一样的输出。
//
// 平台差异全部收敛在 pty_windows.go / pty_unix.go 里，业务层只依赖 Session 接口，
// 不允许出现 runtime.GOOS 判断。
package pty

import (
	"errors"
	"io"
)

// ErrNotImplemented 表示当前平台实现尚未接入。
var ErrNotImplemented = errors.New("伪终端尚未实现")

// Options 是启动终端时的参数。
type Options struct {
	// Command 是用户配置里的完整命令，交给 shell 执行（Windows: cmd.exe /c）。
	// 不拆成 argv，因为配置里常见 `pnpm dev && echo ok` 这种复合命令。
	Command string
	Dir     string   // 工作目录
	Env     []string // 完整环境变量，nil 表示继承父进程
	Cols    int
	Rows    int

	// JobHandle 是进程树容器句柄，由 proctree 创建。
	//
	// 非 0 时会在创建进程的同一次 CreateProcess 调用里挂上，保证进程一出生
	// 就受管，中间没有「先启动再 assign」的竞态窗口。Unix 忽略此字段。
	JobHandle uintptr
}

// Session 是一个已经启动的伪终端会话。
type Session interface {
	io.ReadWriteCloser

	// Resize 同步终端尺寸，前端拖动日志面板时调用。
	Resize(cols, rows int) error

	// Pid 返回 shell 进程号。
	Pid() int

	// Wait 阻塞到进程退出，返回退出码。可被多个 goroutine 同时调用，
	// 只有第一次会真正等待，后续直接拿缓存结果。
	Wait() (int, error)
}
