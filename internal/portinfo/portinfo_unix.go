//go:build !windows

package portinfo

// 待接入：解析 /proc/net/tcp 和 /proc/net/tcp6 拿 inode，再扫 /proc/<pid>/fd
// 反查 PID；macOS 走 lsof 或 sysctl。M1 只面向 Windows，先返回空快照——
// 界面会退化成「不知道端口有没有被占」，不会报错。
func snapshotListeners() map[int]uint32 {
	return nil
}

func processName(pid uint32) string {
	return ""
}
