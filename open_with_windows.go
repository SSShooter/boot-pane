package main

import (
	"os/exec"
	"strings"
	"syscall"
)

// startScriptFile 启动 .cmd/.bat 垫片。
//
// Windows 的 CreateProcess 不认批处理文件，必须借 cmd /c 转一手。这里不用
// exec.Command("cmd", "/c", ...) 让 os/exec 自动拼命令行：它那一套转义规则
// 是给 C 运行时准备的，cmd 会把生成的引号吃掉，带空格的路径被截成
// `C:\Program`，报「不是内部或外部命令」。所以直接给 cmd 一段自己拼好的
// 命令行，并用 /s 让它把这串原样当成命令处理。
func startScriptFile(bin string, args []string) error {
	parts := make([]string, 0, len(args)+1)
	// 程序路径始终加引号：第一次进来的路径很可能带空格。
	parts = append(parts, `"`+bin+`"`)
	for _, arg := range args {
		parts = append(parts, quoteCmdArg(arg))
	}

	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `/s /c "` + strings.Join(parts, " ") + `"`,
	}
	return cmd.Start()
}

// quoteCmdArg 按 cmd 的习惯给参数加引号：空参数或含空白时必须包起来。
func quoteCmdArg(s string) string {
	if s == "" || strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
