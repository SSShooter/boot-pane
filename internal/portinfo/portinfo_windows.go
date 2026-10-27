//go:build windows

package portinfo

import "bootpane/internal/win32"

// snapshotListeners 拿一份「端口 → PID」的全量快照。
func snapshotListeners() map[int]uint32 {
	return win32.ListeningPorts()
}

func processName(pid uint32) string {
	return win32.ProcessName(pid)
}
