//go:build !windows

package proctree

import (
	"errors"
	"sync"
	"syscall"

	"bootpane/internal/procinfo"
)

// New 创建 Unix 进程组容器。
//
// Unix 没有 Windows Job Object 那种「先建容器、再生进程」的原语：进程组
// 归属是 fork 时由 SysProcAttr（Setsid）一次性决定的。所以这里返回的只是
// 个空壳，shell 启动成功后由 Manager 调 Attach 把组 ID（== shell 的 PID）
// 登记进来。这个先后顺序由 process.Manager 的 Start 链路保证。
func New() (Group, error) {
	return &unixGroup{}, nil
}

type unixGroup struct {
	mu   sync.Mutex
	pgid int // 进程组 ID，即被 Attach 的 shell PID；0 表示还没有成员
}

// Handle Unix 靠启动时的 SysProcAttr 定进程组，没有容器句柄可挂载，
// 恒返回 0（pty 侧见到 0 就知道不用做挂载动作）。
func (g *unixGroup) Handle() uintptr { return 0 }

// Attach 登记进程组 ID。
//
// pty_unix 启动 shell 时用 Setsid 让它自成一个会话和进程组，组长 PID
// 就是组 ID，所以这里直接拿会话 PID 当 pgid。
func (g *unixGroup) Attach(pid int) {
	if pid <= 0 {
		return
	}
	g.mu.Lock()
	g.pgid = pid
	g.mu.Unlock()
}

// Kill 对整个进程组发 SIGKILL。
//
// 和 Windows 的 TerminateJobObject 语义对齐：一次调用清掉全部成员，
// 包括 shell 已经后台化的孙进程——只要它们没自己换进程组就逃不掉。
// 组内进程已经全退（ESRCH）不算错误，保证幂等。
func (g *unixGroup) Kill() error {
	g.mu.Lock()
	pgid := g.pgid
	g.mu.Unlock()
	if pgid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// Usage 汇总进程组里所有还活着的成员的资源占用。
//
// 与 Windows Job 的差异：Job 的 CPU 时间包含已退出的子进程，这里只能看
// 到活着的成员。上层按两次采样的差值算占用率，差值语义不受影响；
// 内存的「当前工作集之和」两边口径一致。
func (g *unixGroup) Usage() (Usage, error) {
	g.mu.Lock()
	pgid := g.pgid
	g.mu.Unlock()
	if pgid <= 0 {
		return Usage{}, nil
	}

	procs, err := procinfo.List()
	if err != nil {
		return Usage{}, err
	}

	var u Usage
	for _, p := range procs {
		if p.PGID != pgid || !p.Live {
			continue
		}
		cpu, rss, err := procinfo.Rusage(p.PID)
		if err != nil {
			continue // 枚举和查询之间进程可能刚好退出，跳过即可
		}
		u.CPUTime += cpu
		u.Memory += rss
		u.Processes++
	}
	return u, nil
}

// Close Unix 上没有需要内核帮我们兜底的容器句柄，进程组随进程生死，
// 这里无事可做。
func (g *unixGroup) Close() error { return nil }
