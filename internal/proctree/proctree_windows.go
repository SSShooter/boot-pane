//go:build windows

package proctree

import (
	"sync"
	"syscall"
	"time"

	"bootpane/internal/win32"
)

// New 创建一个 Windows Job Object 作为进程树容器。
//
// 建完立刻打开 KILL_ON_JOB_CLOSE：只要本进程还活着，句柄就一直持有；
// 一旦 bootpane 异常退出，内核会替我们清掉 Job 里所有进程，不会留下
// 占着端口的孤儿 node。
func New() (Group, error) {
	h, err := win32.CreateJobObject()
	if err != nil {
		return nil, err
	}
	if err := win32.SetKillOnJobClose(h); err != nil {
		_ = win32.CloseHandle(h)
		return nil, err
	}
	return &windowsGroup{handle: uintptr(h)}, nil
}

type windowsGroup struct {
	handle uintptr

	killOnce  sync.Once
	killErr   error
	closeOnce sync.Once
	closeErr  error
}

func (g *windowsGroup) Handle() uintptr { return g.handle }

// Attach Windows 上进程出生即在 Job 里，没有「启动后再登记」这一步。
func (g *windowsGroup) Attach(int) {}

// Kill 用 TerminateJobObject 一次性清空整个 Job。
//
// 比 EnumProcesses + 逐个 TerminateProcess 可靠：后者要自己维护父子关系，
// 遇到已经 reparent 的进程就断了，而且中间新 fork 出来的进程会漏掉。
func (g *windowsGroup) Kill() error {
	g.killOnce.Do(func() {
		if g.handle == 0 {
			return
		}
		g.killErr = win32.TerminateJobObject(handleOf(g.handle), 1)
	})
	return g.killErr
}

func (g *windowsGroup) Usage() (Usage, error) {
	var u Usage
	if g.handle == 0 {
		return u, ErrNotImplemented
	}

	acct, err := win32.QueryJobAccounting(handleOf(g.handle))
	if err != nil {
		return u, err
	}
	// TotalUserTime / TotalKernelTime 单位是 100ns，直接转成 Duration。
	u.CPUTime = time.Duration(acct.Basic.TotalUserTime+acct.Basic.TotalKernelTime) * 100 * time.Nanosecond

	pids, err := win32.QueryJobPids(handleOf(g.handle))
	if err != nil {
		// CPU 已经拿到了，内存查不到不算致命，返回部分结果。
		return u, nil
	}
	u.Processes = len(pids)
	for _, pid := range pids {
		// 进程可能在枚举和查询之间就退出了，跳过即可。
		if mem, err := win32.ProcessWorkingSet(pid); err == nil {
			u.Memory += mem
		}
	}
	return u, nil
}

func (g *windowsGroup) Close() error {
	g.closeOnce.Do(func() {
		if g.handle == 0 {
			return
		}
		g.closeErr = win32.CloseHandle(handleOf(g.handle))
		g.handle = 0
	})
	return g.closeErr
}

// handleOf 把 uintptr 转回 syscall.Handle，集中一处避免到处写转换。
func handleOf(h uintptr) syscall.Handle {
	return syscall.Handle(h)
}
