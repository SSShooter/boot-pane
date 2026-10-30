//go:build !windows

package prockill

import (
	"errors"
	"syscall"

	"bootpane/internal/procinfo"
)

// tree 沿父子关系收集 root 及其所有子孙进程，子进程排在父进程之前。
//
// Windows 用 Toolhelp 快照摸父子关系，Unix 这里换 procinfo 的全量快照，
// 逻辑相同：从 root 出发按 PPID 建 children 索引，后序遍历——先收完所有
// 子孙再收自己，正好是「先杀子再杀父」需要的顺序。
//
// root 已经不存在时返回空列表不算错误：重启场景下目标脚本可能刚好跑完。
func tree(root int) ([]Killed, error) {
	procs, err := procinfo.List()
	if err != nil {
		return nil, err
	}

	children := make(map[int][]int, len(procs))
	live := make(map[int]procinfo.Proc, len(procs))
	for _, p := range procs {
		if !p.Live || p.PID <= 0 {
			continue
		}
		live[p.PID] = p
		children[p.PPID] = append(children[p.PPID], p.PID)
	}
	if _, ok := live[root]; !ok {
		return nil, nil
	}

	// visited 防环：正常的进程树不会有环，但内核快照和用户态视图之间
	// 存在时间差，防御一下总没错。
	visited := make(map[int]bool)
	var order []int
	var visit func(pid int)
	visit = func(pid int) {
		if visited[pid] {
			return
		}
		visited[pid] = true
		for _, c := range children[pid] {
			visit(c)
		}
		order = append(order, pid)
	}
	visit(root)

	out := make([]Killed, 0, len(order))
	for _, pid := range order {
		p := live[pid]
		out = append(out, Killed{PID: pid, Name: p.Name})
	}
	return out, nil
}

// kill 按 tree 的顺序（子在前父在后）逐个 SIGKILL。
//
// 不走 SIGTERM 等退出的温柔路线，和 Windows 的 TerminateProcess 对齐：
// 走到重启流程说明旧实例已经不受控，多等一秒就多一秒端口冲突的风险。
// 已经自行退出的进程跳过，不算失败。
func kill(root int) ([]Killed, error) {
	victims, err := tree(root)
	if err != nil {
		return nil, err
	}

	killed := make([]Killed, 0, len(victims))
	for _, v := range victims {
		if err := syscall.Kill(v.PID, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				continue
			}
			return killed, err
		}
		killed = append(killed, v)
	}
	return killed, nil
}
