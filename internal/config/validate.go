package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Validate 校验合并后的项目配置，返回所有发现的问题。
//
// 校验分两档：error 会阻止启动（命令为空、依赖不存在、依赖成环），
// warn 只提示（端口重复这类可能是故意的场景）。
func Validate(rp *ResolvedProject) []Issue {
	var issues []Issue
	if !rp.Exists {
		// 目录本身就不在，逐条校验任务只会刷屏，没意义。
		return issues
	}

	ids := make(map[string]Task, len(rp.Tasks))
	for _, t := range rp.Tasks {
		if t.ID == "" {
			issues = append(issues, Issue{Level: "error", Message: fmt.Sprintf("任务 %q 缺少 id", t.Label)})
			continue
		}
		if _, dup := ids[t.ID]; dup {
			issues = append(issues, Issue{Level: "error", TaskID: t.ID, Message: "任务 id 重复"})
			continue
		}
		ids[t.ID] = t
	}

	ports := make(map[int]string)

	for _, t := range rp.Tasks {
		if t.ID == "" {
			continue
		}
		if strings.TrimSpace(t.Command) == "" {
			issues = append(issues, Issue{Level: "error", TaskID: t.ID, Message: "未配置启动命令"})
		}
		if t.Cwd != "" {
			abs := filepath.Join(rp.Path, t.Cwd)
			if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
				issues = append(issues, Issue{Level: "error", TaskID: t.ID, Message: fmt.Sprintf("工作目录不存在: %s", t.Cwd)})
			}
		}
		for _, dep := range t.DependsOn {
			if _, ok := ids[dep]; !ok {
				issues = append(issues, Issue{Level: "error", TaskID: t.ID, Message: fmt.Sprintf("依赖的任务 %q 不存在", dep)})
			}
		}
		if wf := t.WaitFor; wf != nil {
			switch wf.Type {
			case WaitPort:
				if wf.Port <= 0 || wf.Port > 65535 {
					issues = append(issues, Issue{Level: "error", TaskID: t.ID, Message: "waitFor.port 不是合法端口"})
				} else if other, dup := ports[wf.Port]; dup {
					issues = append(issues, Issue{Level: "warn", TaskID: t.ID, Message: fmt.Sprintf("端口 %d 与任务 %q 重复", wf.Port, other)})
				} else {
					ports[wf.Port] = t.ID
				}
			case WaitLog:
				if strings.TrimSpace(wf.Log) == "" {
					issues = append(issues, Issue{Level: "error", TaskID: t.ID, Message: "waitFor.type 为 log 但未填写关键字"})
				}
			default:
				issues = append(issues, Issue{Level: "error", TaskID: t.ID, Message: fmt.Sprintf("未知的 waitFor.type: %q", wf.Type)})
			}
		}
	}

	for _, pl := range rp.Pipelines {
		label := pl.Label
		if label == "" {
			label = pl.ID
		}
		for _, id := range pl.Tasks {
			if _, ok := ids[id]; !ok {
				issues = append(issues, Issue{Level: "error", Message: fmt.Sprintf("编排 %q 引用了不存在的任务 %q", label, id)})
			}
		}
	}

	return append(issues, detectCycles(ids)...)
}

// detectCycles 用三色标记找出依赖环。成环时后面的启动顺序无法确定，必须报错。
func detectCycles(tasks map[string]Task) []Issue {
	const (
		white = iota
		gray
		black
	)
	color := make(map[string]int, len(tasks))
	var issues []Issue
	var stack []string

	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = gray
		stack = append(stack, id)
		for _, dep := range tasks[id].DependsOn {
			if _, ok := tasks[dep]; !ok {
				continue
			}
			switch color[dep] {
			case gray:
				path := append(append([]string{}, stack...), dep)
				issues = append(issues, Issue{
					Level:   "error",
					TaskID:  id,
					Message: "依赖成环: " + strings.Join(path, " → "),
				})
				return true
			case white:
				if visit(dep) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	keys := make([]string, 0, len(tasks))
	for id := range tasks {
		keys = append(keys, id)
	}
	sort.Strings(keys) // 保证报错信息稳定可复现

	for _, id := range keys {
		if color[id] == white && visit(id) {
			break
		}
	}
	return issues
}
