//go:build darwin

package pty

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPty 在 macOS 上分配一对伪终端，返回主端和从端路径。
//
// macOS 的 openpty 在 libutil 里，Go 标准库和 x/sys 都没有封装；它内部
// 就是「打开 /dev/ptmx → 授权从端 → 解锁从端 → 查从端路径」四步 ioctl，
// 自己走一遍连 cgo 都省了。
func openPty() (*os.File, string, error) {
	// /dev/ptmx 每次 open 都会分配一对新的主从端。Go 的 os.OpenFile 自带
	// O_CLOEXEC，主端不会被意外继承到子进程——这正是我们想要的。
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	fd := int(master.Fd())

	// TIOCPTYGRANT 授权 + TIOCPTYUNLK 解锁，等价于 grantpt/unlockpt。
	// 不解锁的话 slave 打开时会一直阻塞到有进程读主端。
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGRANT, 0); errno != 0 {
		master.Close()
		return nil, "", fmt.Errorf("授权从端失败: %w", errno)
	}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYUNLK, 0); errno != 0 {
		master.Close()
		return nil, "", fmt.Errorf("解锁从端失败: %w", errno)
	}

	// 从端路径固定是 /dev/ttysN，但编号只能问内核（TIOCPTYGNAME 写回
	// NUL 结尾的路径）。
	var buf [128]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		master.Close()
		return nil, "", fmt.Errorf("解析从端路径失败: %w", errno)
	}
	name := string(buf[:bytes.IndexByte(buf[:], 0)])
	if name == "" {
		master.Close()
		return nil, "", fmt.Errorf("从端路径为空")
	}
	return master, name, nil
}
