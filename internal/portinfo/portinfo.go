// Package portinfo 查询本机端口的监听进程。
//
// 存在的理由：bootpane 只能管自己启动的进程树，管不了用户手动起的、或者上次
// 遗留的进程。用户点「启动」时脚本经常自己判断「已在运行，跳过」然后退出，
// 界面就会显示「已停止」，但服务其实活得好好的。这里把「端口现在归谁」单独
// 查出来，让界面能如实告诉用户，而不是只反映 bootpane 自己的进程状态。
package portinfo

// Listener 描述一个端口的监听情况。
type Listener struct {
	Port      int    `json:"port"`
	Listening bool   `json:"listening"`
	PID       int    `json:"pid"`     // 监听进程的 PID，Listening 为 false 时为 0
	Process   string `json:"process"` // 可执行文件名，如 node.exe；查不到时为空
}

// LookupAll 批量查询端口占用。
//
// 只做一次全表快照再按端口匹配，而不是每个端口查一次系统调用——
// 界面一次可能要问十来个端口，逐个查会明显拖慢。
func LookupAll(ports []int) []Listener {
	if len(ports) == 0 {
		return nil
	}

	listening := snapshotListeners()
	out := make([]Listener, 0, len(ports))
	seen := make(map[int]bool, len(ports))

	for _, port := range ports {
		if port <= 0 || port > 65535 || seen[port] {
			continue
		}
		seen[port] = true

		l := Listener{Port: port}
		if pid, ok := listening[port]; ok {
			l.Listening = true
			l.PID = int(pid)
			l.Process = processName(pid)
		}
		out = append(out, l)
	}
	return out
}

// Lookup 查询单个端口。
func Lookup(port int) Listener {
	all := LookupAll([]int{port})
	if len(all) == 0 {
		return Listener{Port: port}
	}
	return all[0]
}
