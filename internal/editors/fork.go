package editors

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// codeForkMarkers 是 VS Code 家族 CLI 垫片里必然出现的两段内容。
//
// 所有分支（Cursor / Windsurf / Trae / Kiro / Antigravity …）都是改个名字
// 重新打包的 Electron 应用，垫片本身几乎一模一样：设置 ELECTRON_RUN_AS_NODE
// 后让 Electron 去跑 resources/app/out/cli.js。用这两段做指纹，比维护一张
// 「谁家 CLI 叫什么」的名单可靠得多——名单永远赶不上新分支。
var codeForkMarkers = []string{"ELECTRON_RUN_AS_NODE", "cli.js"}

// scanCodeForks 在若干应用安装目录下扫 VS Code 家族的 CLI 垫片。
//
// 只认固定几层布局，不做全盘遍历：垫片内容必须同时含 codeForkMarkers，
// 才算得上「能打开目录的编辑器 CLI」，避免把随便一个 .cmd 当成编辑器。
func scanCodeForks(roots []string) []Editor {
	var list []Editor
	for _, root := range roots {
		if root == "" {
			continue
		}
		apps, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, app := range apps {
			if !app.IsDir() {
				continue
			}
			dir := filepath.Join(root, app.Name())
			for _, pattern := range forkLayouts() {
				matches, _ := filepath.Glob(filepath.Join(dir, pattern))
				for _, shim := range matches {
					if !isCodeForkShim(shim) {
						continue
					}
					name := forkName(app.Name())
					list = append(list, Editor{
						ID:      "scan:" + strings.ToLower(strings.ReplaceAll(name, " ", "-")),
						Name:    name,
						Command: quotePath(shim),
						Source:  SourceScan,
					})
				}
			}
		}
	}
	return list
}

// isCodeForkShim 用垫片内容判断它是不是 VS Code 家族的 CLI。
//
// 只看开头几 KB：垫片都很短，读全文没有必要，而有些安装目录里同名文件
// 其实是几十 MB 的 Electron 主程序。
func isCodeForkShim(shim string) bool {
	f, err := os.Open(shim)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	head := string(buf[:n])
	for _, marker := range codeForkMarkers {
		if !strings.Contains(head, marker) {
			return false
		}
	}
	return true
}

// forkName 从安装目录名推出展示名。
//
// 目录名就是品牌名（Kiro、Antigravity IDE），比垫片文件名（kiro、
// antigravity-ide）更适合出现在下拉框里。macOS 的 .app 后缀要去掉。
func forkName(dir string) string {
	return strings.TrimSuffix(dir, ".app")
}

// forkLayouts 返回各平台上 VS Code 家族放 CLI 的相对路径。
func forkLayouts() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join("bin", "*.cmd"),
			filepath.Join("resources", "app", "bin", "*.cmd"),
		}
	case "darwin":
		return []string{
			filepath.Join("Contents", "Resources", "app", "bin", "*"),
		}
	default:
		return []string{
			filepath.Join("bin", "*"),
			filepath.Join("resources", "app", "bin", "*"),
		}
	}
}

// forkRoots 返回各平台上可能装着编辑器的应用目录。
func forkRoots() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			under(os.Getenv("LOCALAPPDATA"), "Programs"),
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		return []string{"/Applications", under(home, "Applications")}
	default:
		home, _ := os.UserHomeDir()
		return []string{
			"/usr/share",
			"/opt",
			under(home, ".local", "share"),
		}
	}
}
