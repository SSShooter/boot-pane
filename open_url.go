package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"bootpane/internal/config"
)

// openExternalURL 用系统默认浏览器打开网页地址。
//
// 只放行 http/https：配置来自 .bootpane.json 或大模型产出，不能让它有机会
// 通过 file:// 或自定义协议在机器上拉起任意程序。
func openExternalURL(raw string) error {
	if !config.IsHTTPURL(raw) {
		return fmt.Errorf("页面地址无效: %q（只支持 http/https）", raw)
	}
	bin, args := browserCommand(raw)
	return exec.Command(bin, args...).Start()
}

// browserCommand 返回各平台「用默认浏览器打开 URL」的命令。
//
// Windows 用 rundll32 而不是 `cmd /c start`：后者会重新解析一遍命令行，
// 地址里带 & 或空格时会被截断。rundll32 直接把 URL 当参数收，不经过 shell。
func browserCommand(raw string) (string, []string) {
	switch runtime.GOOS {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", raw}
	case "darwin":
		return "open", []string{raw}
	default:
		return "xdg-open", []string{raw}
	}
}
