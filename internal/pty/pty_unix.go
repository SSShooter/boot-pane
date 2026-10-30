//go:build !windows

package pty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// Start 在 Unix 上通过伪终端启动一个终端会话。
//
// 关键点：slave 同时充当 stdin/stdout/stderr，并用 Setsid + Setctty 把它
// 设成控制终端，shell 眼里的环境和真实终端一模一样（颜色、spinner、
// 进度条都在）。shell 自成一个会话和进程组（组长，pgid == PID），所以
// proctree 那边拿到 PID 就等于拿到了整棵树的组 ID。
//
// 调用契约：返回的 Session 必须有人持续 Read，直到读到错误为止。
// 登录 shell（-l）退出时用 TCSADRAIN 恢复终端设置，会等输出队列排空；
// master 无人读的话 shell 会永远卡在退出路径上，Wait 也就永远不返回。
// Manager 的日志泵从启动起就在读，满足这个契约；Windows 的 ConPTY 同样
// 要求持续读，两边行为对齐。
func Start(opts Options) (Session, error) {
	cols, rows := opts.Cols, opts.Rows
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}

	master, slaveName, err := openPty()
	if err != nil {
		return nil, fmt.Errorf("创建伪终端失败: %w", err)
	}
	// 中途任何一步失败都从这里收尾，避免泄漏主端 fd。
	fail := func(err error) (Session, error) {
		master.Close()
		return nil, err
	}

	// 打开从端。O_NOCTTY：控制终端的归属由子进程的 Setctty 决定，
	// 不让本进程抢先认领。
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return fail(fmt.Errorf("打开从端失败: %w", err))
	}

	// 启动前把窗口尺寸设好，否则第一屏日志按 0x0 布局折行。
	ws := unix.Winsize{Row: uint16(rows), Col: uint16(cols)}
	_ = unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &ws)

	shell := userShell()
	cmd := exec.Command(shell, shellArgv(shell, opts.Command)...)
	cmd.Dir = opts.Dir
	cmd.Env = ensurePath(opts.Env)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// 新会话 + 新进程组：shell 成为组长，pgid == PID，proctree
		// 靠这个约定对 -pgid 发信号清整棵树。
		Setsid: true,
		// slave 在子进程里是 fd 0（stdin），把它认作控制终端。
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		slave.Close()
		return fail(fmt.Errorf("启动进程失败: %w", err))
	}

	// 子进程已经拿到自己的 slave。本进程这份留着读不出东西，只会阻止
	// master 的 read 在整棵进程树退出后返回 EIO，必须立刻关掉。
	slave.Close()

	return &unixSession{cmd: cmd, master: master, pid: cmd.Process.Pid}, nil
}

// ShellDescription 说明命令实际被哪个 shell 执行。
//
// 生成配置的 prompt 需要把这件事告诉大模型（POSIX sh 不认 `cmd1 && cmd2`
// 之外的部分语法差异），但业务层按约定不许判断 runtime.GOOS，所以由这里
// 按平台给一句话，调用方直接用。
func ShellDescription() string {
	return "用户默认 shell 的登录模式（$SHELL -l -c，取不到时退回 /bin/sh -c）"
}

// userShell 返回执行命令用的 shell。
//
// 优先用用户的登录 shell：pnpm、node 这类工具常由 homebrew/nvm 装在只在
// shell 配置里导出的路径下，用 /bin/sh 直接跑会找不到命令。GUI 启动时
// SHELL 一般也有值（launchd 会带），实在取不到就退回 /bin/sh。
func userShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// shellArgv 把用户命令包成 shell 参数。
//
// 登录模式（-l）会加载 .zprofile / .bash_profile，让 PATH 等环境变量和
// 用户在终端里看到的一致。dash 这类 POSIX sh 不认 -l，按名字排除，
// 免得还没跑到命令就先报 Illegal option。
func shellArgv(shell, command string) []string {
	argv := []string{}
	switch filepath.Base(shell) {
	case "zsh", "bash", "fish", "ksh", "mksh", "tcsh":
		argv = append(argv, "-l")
	}
	return append(argv, "-c", command)
}

// ensurePath 给子进程环境补上 GUI 启动时常缺的可执行目录。
//
// 从 Finder / launchd 启动的应用只继承 /usr/bin:/bin:/usr/sbin:/sbin 这套
// 极简 PATH，用户在终端里随手就能跑的 pnpm 在这里会直接 not found。
// 登录 shell（-l）能补回大部分，但依赖 shell 配置文件写对；这里再兜一层，
// 把常见包管理器的 bin 目录追加到 PATH 末尾——已存在的跳过，用户自己的
// 条目优先级不受影响。
func ensurePath(env []string) []string {
	if env == nil {
		env = os.Environ()
	}

	extra := []string{
		"/opt/homebrew/bin", "/opt/homebrew/sbin", // Apple Silicon Homebrew
		"/usr/local/bin", "/usr/local/sbin", // Intel Homebrew / 常见手动安装位置
		"/usr/local/go/bin",
	}
	if home, err := os.UserHomeDir(); err == nil {
		extra = append(extra,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "bin"),
			filepath.Join(home, "go", "bin"),
		)
	}

	const prefix = "PATH="
	for i, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			continue
		}
		have := make(map[string]bool)
		for _, d := range filepath.SplitList(kv[len(prefix):]) {
			have[d] = true
		}
		missing := make([]string, 0, len(extra))
		for _, d := range extra {
			if !have[d] {
				missing = append(missing, d)
			}
		}
		if len(missing) == 0 {
			return env
		}
		out := make([]string, len(env))
		copy(out, env)
		out[i] = kv + ":" + strings.Join(missing, ":")
		return out
	}
	// 环境里根本没有 PATH：补一条最小可用值。
	return append(env, "PATH="+strings.Join(extra, ":"))
}

type unixSession struct {
	cmd    *exec.Cmd
	master *os.File
	pid    int

	waitOnce sync.Once
	exitCode int
	waitErr  error

	closeOnce sync.Once
}

func (s *unixSession) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return s.master.Read(p)
}

func (s *unixSession) Write(p []byte) (int, error) {
	return s.master.Write(p)
}

// Resize 同步伪终端的窗口尺寸。拖动日志面板时由前端触发。
func (s *unixSession) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 || s.master == nil {
		return nil
	}
	ws := unix.Winsize{Row: uint16(rows), Col: uint16(cols)}
	return unix.IoctlSetWinsize(int(s.master.Fd()), unix.TIOCSWINSZ, &ws)
}

func (s *unixSession) Pid() int { return s.pid }

// Wait 阻塞到 shell 进程退出。
//
// 多个 goroutine 同时调用是安全的：sync.Once 保证只有一个真的在等，
// 其余阻塞在 Once 上，拿到的是同一份结果。
//
// 错误语义和 Windows 版对齐：正常结束（哪怕退出码非 0，比如命令不存在
// 退出 127）只返回码不带错；被信号杀死的退出码是 -1，此时保留错误，
// 上层能拿到 "signal: killed" 这类原因。
func (s *unixSession) Wait() (int, error) {
	s.waitOnce.Do(func() {
		err := s.cmd.Wait()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			s.exitCode = ee.ExitCode()
			if ee.ExitCode() >= 0 {
				err = nil
			}
		}
		if err != nil {
			s.waitErr = err
		}
	})
	return s.exitCode, s.waitErr
}

// Close 关闭 master。
//
// Unix 上没有 ConPTY 那种「关掉顺带杀进程」的语义，清树完全靠 proctree
// 对进程组发信号；这里只需要释放 fd，让还阻塞在 Read 上的 goroutine
// 立刻拿到错误返回。调用方必须保证此时没有别的 goroutine 在用 master
// （见 Manager 的清理顺序）。
func (s *unixSession) Close() error {
	s.closeOnce.Do(func() {
		s.master.Close()
	})
	return nil
}
