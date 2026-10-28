package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// openWithPlaceholder 是命令里代表项目目录的占位符。
//
// 多数编辑器 CLI 就是「程序 + 目录」，所以占位符可以不写；写了则替换成真实
// 路径，用来覆盖 `--new-window {path}` 这种需要指定参数位置的用法。
const openWithPlaceholder = "{path}"

// splitCommandLine 按空格拆分命令行，支持用双引号包住带空格的路径。
//
// 不引 shell 解析：设置里存的是「程序 + 参数」，不是一段要交给 shell 的脚本。
// 走 shell 既多一层引号规则，也会让 `&`、`|` 这类字符被意外解释。
func splitCommandLine(s string) []string {
	var (
		out     []string
		cur     strings.Builder
		inQuote bool
		started bool
	)

	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			started = true // 空引号 `` 也算一个参数
		case (r == ' ' || r == '\t') && !inQuote:
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if started {
		out = append(out, cur.String())
	}
	return out
}

// lookupOpenWith 定位命令里的可执行文件。
//
// 保存设置时就会调一次：命令拼错应该在保存那一刻就报出来，而不是等用户去点
// 「打开目录」才失败——那时错误离输入框已经很远，用户很难联想到是设置的问题。
func lookupOpenWith(command string) (string, error) {
	tokens := splitCommandLine(command)
	if len(tokens) == 0 {
		return "", errors.New("打开方式不能为空")
	}
	bin, err := exec.LookPath(tokens[0])
	if err != nil {
		return "", fmt.Errorf("找不到命令 %q，请确认它已安装并在 PATH 里", tokens[0])
	}
	return bin, nil
}

// resolveOpenWith 把保存的命令行展开成「可执行文件 + 参数」。
func resolveOpenWith(command, path string) (bin string, args []string, err error) {
	bin, err = lookupOpenWith(command)
	if err != nil {
		return "", nil, err
	}

	args = splitCommandLine(command)[1:]
	replaced := false
	for i, arg := range args {
		if strings.Contains(arg, openWithPlaceholder) {
			args[i] = strings.ReplaceAll(arg, openWithPlaceholder, path)
			replaced = true
		}
	}
	if !replaced {
		args = append(args, path)
	}
	return bin, args, nil
}

// launchOpenWith 用配置的命令打开项目目录。
//
// 只负责把进程拉起来，不等它退出：编辑器是长驻进程，等它等于把界面卡住。
func launchOpenWith(command, path string) error {
	bin, args, err := resolveOpenWith(command, path)
	if err != nil {
		return err
	}
	if isScriptFile(bin) {
		return startScriptFile(bin, args)
	}
	return exec.Command(bin, args...).Start()
}

// isScriptFile 判断是不是必须由 cmd.exe 解释的批处理垫片。
//
// VS Code、Cursor 这些在 Windows 上的 CLI 其实是 .cmd 文件，
// CreateProcess 不能直接执行，直接 Start 会得到「不是内部或外部命令」。
func isScriptFile(bin string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	switch strings.ToLower(filepath.Ext(bin)) {
	case ".cmd", ".bat":
		return true
	}
	return false
}
