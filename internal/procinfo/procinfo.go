// Package procinfo 枚举本机进程并查询单个进程的资源占用。
//
// 存在的理由：proctree 的 Usage（按进程组聚合 CPU / 内存）和 prockill 的
// 树遍历（按父子关系爬树）都需要一份全量进程快照，但两者聚合方式不同，
// 枚举本身抽成独立一层。平台差异收敛在 procinfo_darwin.go（sysctl）和
// procinfo_linux.go（/proc）里，调用方不做任何 GOOS 判断。
package procinfo

// Proc 是一次全量枚举里的单个进程快照。
type Proc struct {
	PID  int
	PPID int
	PGID int
	Name string // 可执行文件短名，如 node；来自内核的定长字段，约 16 字节
	Live bool   // 排除已经变成 zombie、只等父进程收尸的进程
}

// List 返回当前所有进程的快照。
//
// 快照是某一瞬间的一致视图：返回之后进程可能已经退出，也可能刚好新起了
// 进程，调用方要按「可能略有过期」来消费。顺序不保证。
//
// 平台实现：darwin 走 sysctl kern.proc.all，一次内核调用拿全量；
// linux 逐个读 /proc/<pid>/stat。

// Rusage 返回单个进程的累计 CPU 时间（用户态 + 内核态）和当前常驻内存。
//
// 累计 CPU 时间不包含已退出的子进程——这一点和 Windows Job Object 的
// 全树累计不同，上层用两次采样的差值算占用率，语义上是兼容的。
// 进程已退出时返回错误，调用方跳过即可。
//
// 两个函数的具体实现按平台拆在 procinfo_darwin.go / procinfo_linux.go，
// 这里只放类型和约定。
