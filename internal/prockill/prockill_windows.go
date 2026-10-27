//go:build windows

package prockill

import (
	"errors"
	"fmt"
	"os"

	"bootpane/internal/win32"
)

// 进程树的深度上限。正常进程树不会有这么深，设上限纯粹是防御异常数据。
const maxTreeDepth = 64

func tree(root int) ([]Killed, error) {
	if root <= 0 {
		return nil, fmt.Errorf("无效的进程 ID: %d", root)
	}

	nodes, err := win32.SnapshotProcesses()
	if err != nil {
		return nil, err
	}

	children := make(map[int][]win32.ProcessNode, len(nodes))
	names := make(map[int]string, len(nodes))
	exists := false
	for _, n := range nodes {
		children[n.PPID] = append(children[n.PPID], n)
		names[n.PID] = n.Name
		if n.PID == root {
			exists = true
		}
	}
	if !exists {
		return nil, fmt.Errorf("进程 %d 不存在", root)
	}

	var out []Killed
	visited := make(map[int]bool, 16)

	// 后序遍历：先把所有子孙收进来，最后才是自己。
	var walk func(pid, depth int)
	walk = func(pid, depth int) {
		if depth > maxTreeDepth || visited[pid] {
			return
		}
		visited[pid] = true
		for _, c := range children[pid] {
			walk(c.PID, depth+1)
		}
		out = append(out, Killed{PID: pid, Name: names[pid]})
	}
	walk(root, 0)

	return out, nil
}

func kill(root int) ([]Killed, error) {
	// 兜底保护：无论如何不能把 bootpane 自己杀掉。
	if root == os.Getpid() {
		return nil, errors.New("不能终止 bootpane 自身")
	}

	targets, err := tree(root)
	if err != nil {
		return nil, err
	}

	killed := make([]Killed, 0, len(targets))
	var lastErr error
	for _, t := range targets {
		// 跳过 Idle(0) 和 System(4)，这两个杀不掉也不该碰。
		if t.PID <= 4 {
			continue
		}
		if err := win32.TerminateByPID(t.PID, 1); err != nil {
			// 进程可能在我们遍历之后、终止之前自己退出了，跳过即可。
			lastErr = err
			continue
		}
		killed = append(killed, t)
	}

	// 一个都没杀掉才当失败——部分成功也算重启可以继续。
	if len(killed) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("终止进程 %d 失败: %w", root, lastErr)
		}
		return nil, fmt.Errorf("进程 %d 已经不存在", root)
	}
	return killed, nil
}
