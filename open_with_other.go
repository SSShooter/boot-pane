//go:build !windows

package main

import "os/exec"

// startScriptFile 在非 Windows 平台上不会被调用。
//
// isScriptFile 只在 Windows 上返回 true：macOS 的编辑器 CLI 是普通可执行
// 文件或符号链接，不需要经过 shell。
func startScriptFile(bin string, args []string) error {
	return exec.Command(bin, args...).Start()
}
