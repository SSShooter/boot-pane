//go:build windows

package pty

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"bootpane/internal/win32"
)

// Start 在 Windows 上通过 ConPTY 启动一个终端会话。
//
// 关键点：CreateProcess 只调一次，ConPTY 句柄和 Job Object 句柄都通过
// STARTUPINFOEX 的属性列表传进去。这样进程一出生就同时「接上了伪终端」和
// 「进了进程树容器」，不存在任何逃逸窗口。
func Start(opts Options) (Session, error) {
	cols, rows := opts.Cols, opts.Rows
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}

	// 输入管道：我们写 inWrite，ConPTY 读 inRead。
	// 输出管道：ConPTY 写 outWrite，我们读 outRead。
	inRead, inWrite, err := win32.CreatePipe()
	if err != nil {
		return nil, fmt.Errorf("创建输入管道失败: %w", err)
	}
	outRead, outWrite, err := win32.CreatePipe()
	if err != nil {
		_ = win32.CloseHandle(inRead)
		_ = win32.CloseHandle(inWrite)
		return nil, fmt.Errorf("创建输出管道失败: %w", err)
	}

	hpc, err := win32.CreatePseudoConsole(int16(cols), int16(rows), inRead, outWrite)
	if err != nil {
		_ = win32.CloseHandle(inRead)
		_ = win32.CloseHandle(inWrite)
		_ = win32.CloseHandle(outWrite)
		_ = win32.CloseHandle(outRead)
		return nil, fmt.Errorf("创建伪终端失败: %w", err)
	}

	// 包一层 shell。用 /d 跳过注册表里的 AutoRun，用 /s 让 cmd 走简单的
	// 引号处理规则，否则命令里带引号时会被吞掉一层。
	shell := os.Getenv("COMSPEC")
	if shell == "" {
		shell = `C:\Windows\System32\cmd.exe`
	}
	cmdline := fmt.Sprintf(`"%s" /d /s /c "%s"`, shell, opts.Command)

	pi, err := win32.CreateProcess(win32.ProcSpec{
		AppName:     shell,
		CommandLine: cmdline,
		Dir:         opts.Dir,
		Env:         opts.Env,
		JobHandle:   opts.JobHandle,
		PseudoCon:   hpc,
	})
	if err != nil {
		win32.ClosePseudoConsole(hpc)
		_ = win32.CloseHandle(inRead)
		_ = win32.CloseHandle(outWrite)
		_ = win32.CloseHandle(inWrite)
		_ = win32.CloseHandle(outRead)
		return nil, fmt.Errorf("启动进程失败: %w", err)
	}

	// 这两个是 ConPTY 那一侧的句柄，必须等子进程真正连上之后才能关。
	// 提前关掉的话子进程不会接入伪终端，而是悄悄附着到父进程的控制台上，
	// 表现就是「日志一条都收不到，但命令行却在终端里打出来了」。
	_ = win32.CloseHandle(inRead)
	_ = win32.CloseHandle(outWrite)

	// 主线程句柄我们不用，立刻关掉，否则每启动一次泄漏一个。
	_ = win32.CloseHandle(pi.Thread)

	return &conptySession{
		hpc:  hpc,
		in:   inWrite,
		out:  outRead,
		proc: pi.Process,
		pid:  int(pi.ProcessID),
	}, nil
}

type conptySession struct {
	hpc  uintptr
	in   syscall.Handle // 写入端
	out  syscall.Handle // 读取端
	proc syscall.Handle
	pid  int

	waitOnce sync.Once
	exitCode int
	waitErr  error

	closeOnce sync.Once
}

func (s *conptySession) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return win32.ReadFile(s.out, p)
}

func (s *conptySession) Write(p []byte) (int, error) {
	return win32.WriteFile(s.in, p)
}

func (s *conptySession) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 || s.hpc == 0 {
		return nil
	}
	return win32.ResizePseudoConsole(s.hpc, int16(cols), int16(rows))
}

func (s *conptySession) Pid() int { return s.pid }

// Wait 阻塞到进程退出。
//
// 多个 goroutine 同时调用是安全的：sync.Once 保证只有一个真的在等，
// 其余阻塞在 Once 上，拿到的是同一份结果。
func (s *conptySession) Wait() (int, error) {
	s.waitOnce.Do(func() {
		if _, err := win32.WaitForSingleObject(s.proc, win32.Infinite); err != nil {
			s.waitErr = err
			return
		}
		code, err := win32.GetExitCode(s.proc)
		if err != nil {
			s.waitErr = err
			return
		}
		s.exitCode = int(code)
	})
	return s.exitCode, s.waitErr
}

// Close 释放会话。
//
// 顺序有讲究：先关写入端让子进程收到 EOF，再 ClosePseudoConsole——它会连带
// 终止还挂在上面的进程，同时让阻塞中的 Read 返回 broken pipe。如果反过来先
// 关读取端，正在 ReadFile 的 goroutine 会拿着一个失效句柄，行为未定义。
//
// 调用方必须保证此时已经没有 goroutine 在 Read（见 Manager 的清理顺序）。
func (s *conptySession) Close() error {
	s.closeOnce.Do(func() {
		_ = win32.CloseHandle(s.in)
		s.in = 0
		win32.ClosePseudoConsole(s.hpc)
		s.hpc = 0
		_ = win32.CloseHandle(s.out)
		s.out = 0
		_ = win32.CloseHandle(s.proc)
		s.proc = 0
	})
	return nil
}
