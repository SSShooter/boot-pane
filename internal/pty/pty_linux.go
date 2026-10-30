//go:build linux

package pty

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openPty 在 Linux 上分配一对伪终端，返回主端和从端路径。
//
// 流程和 macOS 一致，只是 ioctl 换成 Linux 风格：TIOCGPTN 问从端编号，
// TIOCSPTLCK 置 0 解锁（等价于 unlockpt）。Linux 的从端固定挂在
// /dev/pts/<n> 下，不用另外查路径。
func openPty() (*os.File, string, error) {
	// Go 的 os.OpenFile 自带 O_CLOEXEC，主端不会被意外继承到子进程。
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	fd := int(master.Fd())

	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, "", fmt.Errorf("解析从端编号失败: %w", err)
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, "", fmt.Errorf("解锁从端失败: %w", err)
	}
	return master, fmt.Sprintf("/dev/pts/%d", n), nil
}
