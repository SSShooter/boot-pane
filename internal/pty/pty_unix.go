//go:build !windows

package pty

// Start 在 Unix 上通过 pty 启动一个终端会话。
//
// 待接入：openpty 拿 master/slave，把 slave 挂到 cmd 的 Stdin/Stdout/Stderr，
// cmd.SysProcAttr 设 Setsid + Setctty，Start 之后关掉 slave，只留 master 读写。
// 这块可以等做 mac/Linux 支持时补，接口已经按最终形态定好。
func Start(opts Options) (Session, error) {
	return nil, ErrNotImplemented
}

// ShellDescription 说明命令实际被哪个 shell 执行，供生成配置的 prompt 使用。
//
// 平台差异收敛在 pty 里，业务层不判断 runtime.GOOS。
func ShellDescription() string {
	return "POSIX sh（-c）"
}
