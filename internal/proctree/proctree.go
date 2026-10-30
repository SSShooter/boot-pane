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
	"time"
)

// ErrNotImplemented 表示当前平台实现尚未接入。
var ErrNotImplemented = errors.New("进程树管理尚未实现")

// Group 是一组需要同生共死的进程。
type Group interface {
	// Handle 返回底层容器的原生句柄，供上层在创建进程时一次性挂载。
	//
	// Windows 返回 Job Object 句柄，pty 会把它塞进 STARTUPINFOEX 属性列表，
	// 保证进程一生出来就在 Job 里。Unix 返回 0——那边靠 Setsid 在启动时
	// 定好进程组，不需要预先挂载。
	Handle() uintptr

	// Attach 把刚启动的 shell 会话登记进组。
	//
	// Windows 上进程在 CreateProcess 的同一次调用里就挂进了 Job Object，
	// 这里是空操作；Unix 没有「先建容器再生进程」的原语，进程组归属由
	// 启动时的 SysProcAttr 一次性定好，pty 启动成功后调这里把组 ID
	// （== shell 的 PID）记下来。Start 链路里 Attach 必须发生在任何
	// Kill 之前，Manager 的实现保证了这一点。
	Attach(pid int)

	// Kill 终止整棵进程树，包括已经脱离父进程的孙子进程。幂等。
	Kill() error

	// Usage 返回整棵树的累计资源占用。
	//
	// 注意是「累计」：CPU 时间包含已经退出的子进程，所以调用方要自己按
	// 两次采样的差值算占用率，不能直接拿绝对值当当前负载。
	Usage() (Usage, error)

	// Close 释放容器句柄。Windows 上因为开了 KILL_ON_JOB_CLOSE，
	// 关闭句柄会顺带杀掉残留进程，这是应用崩溃时的兜底。
	Close() error
}

// Usage 是一次资源采样。
type Usage struct {
	CPUTime   time.Duration // 整棵树累计 CPU 时间（用户态 + 内核态）
	Memory    uint64        // 整棵树当前工作集之和，字节
	Processes int           // 当前活着的进程数
}
