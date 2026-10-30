//go:build !windows

package portinfo

import (
	"os/exec"
	"strconv"
	"strings"

	"bootpane/internal/procinfo"
)

// snapshotListeners 用 lsof 列出本机所有 TCP LISTEN 端口及归属 PID。
//
// macOS 没有可读的 /proc，要把 TCP 表和 pid 关联起来只能解析私有 sysctl
// 结构（布局随内核版本变，不能依赖）或走 lsof。一次进程拉起换全表快照，
// CheckPorts 是低频操作，这个成本可以接受；lsof 不存在或出错时返回空
// 快照，界面退化为「不知道端口有没有被占」，不会报错。
func snapshotListeners() map[int]uint32 {
	out, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "+c0").Output()
	if err != nil {
		return nil
	}
	return parseLsof(string(out))
}

// parseLsof 解析 lsof 输出。
//
// 行格式：COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME (STATE)。
// NAME 里的端口取最后一个冒号之后的部分（IPv4 是 *:3000 / 127.0.0.1:3000，
// IPv6 是 [::]:3000，冒号只出现在端口前）。结尾的 (LISTEN) 状态标记
// 先剥掉再取字段。
func parseLsof(text string) map[int]uint32 {
	listeners := make(map[int]uint32)

	lines := strings.Split(text, "\n")
	if len(lines) > 0 {
		lines = lines[1:] // 首行是表头
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if n := len(fields); n > 0 && strings.HasPrefix(fields[n-1], "(") {
			fields = fields[:n-1]
		}
		if len(fields) < 9 {
			continue
		}
		pid, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil || pid == 0 {
			continue
		}

		name := fields[len(fields)-1]
		idx := strings.LastIndexByte(name, ':')
		if idx < 0 {
			continue
		}
		port, err := strconv.Atoi(name[idx+1:])
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		listeners[port] = uint32(pid)
	}
	return listeners
}

// processName 用 procinfo 的全量快照反查进程名。
//
// sysctl 一次调用就能拿全表，比再拉一次 lsof 便宜；快照略有过期没关系，
// 进程名只是给用户看的辅助信息。
func processName(pid uint32) string {
	procs, err := procinfo.List()
	if err != nil {
		return ""
	}
	for _, p := range procs {
		if p.PID == int(pid) {
			return p.Name
		}
	}
	return ""
}
