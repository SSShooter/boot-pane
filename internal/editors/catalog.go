package editors

import (
	"os"
	"path/filepath"
	"runtime"
)

// catalog 返回当前平台上要探测的编辑器清单。
func catalog() []candidate {
	switch runtime.GOOS {
	case "windows":
		return windowsCatalog()
	case "darwin":
		return darwinCatalog()
	default:
		return linuxCatalog()
	}
}

// under 把若干段拼到 root 下。
//
// root 为空（环境变量缺失，比如父进程没传 LOCALAPPDATA）时返回空串，
// probe 会跳过它，不会去 stat 一个相对路径。
func under(root string, elem ...string) string {
	if root == "" {
		return ""
	}
	return filepath.Join(append([]string{root}, elem...)...)
}

// jetbrains 返回一组 Toolbox 装的 JetBrains IDE 候选。
//
// Toolbox 会给每个 IDE 在 scripts 目录里生成一个命令行垫片，文件名是固定的
// CLI 名（idea / webstorm …），与安装的版本号无关，所以这条路径能跨升级存活。
// ext 是垫片后缀：Windows 是 .cmd，macOS / Linux 没有后缀。
func jetbrains(scripts, ext string) []candidate {
	ides := []struct{ id, name, cli string }{
		{"intellij", "IntelliJ IDEA", "idea"},
		{"webstorm", "WebStorm", "webstorm"},
		{"goland", "GoLand", "goland"},
		{"pycharm", "PyCharm", "pycharm"},
		{"clion", "CLion", "clion"},
		{"phpstorm", "PhpStorm", "phpstorm"},
		{"rubymine", "RubyMine", "rubymine"},
		{"datagrip", "DataGrip", "datagrip"},
		{"rider", "Rider", "rider"},
	}

	list := make([]candidate, 0, len(ides))
	for _, ide := range ides {
		list = append(list, candidate{
			id:    ide.id,
			name:  ide.name,
			clis:  []string{ide.cli},
			paths: []string{under(scripts, ide.cli+ext)},
		})
	}
	return list
}

// windowsCatalog 列出 Windows 上的常见安装位置。
//
// VS Code 系（Cursor / Windsurf / Trae / VSCodium 都是它的分支）的安装布局
// 几乎一样：bin 目录下放一个 .cmd 垫片，只是 CLI 名字不同。这些路径都是
// 「猜」的，猜错只是少一条候选，不影响别的项。
func windowsCatalog() []candidate {
	local := os.Getenv("LOCALAPPDATA")
	programFiles := os.Getenv("ProgramFiles")

	list := []candidate{
		{
			id:   "vscode",
			name: "VS Code",
			clis: []string{"code"},
			paths: []string{
				under(local, "Programs", "Microsoft VS Code", "bin", "code.cmd"),
				under(programFiles, "Microsoft VS Code", "bin", "code.cmd"),
			},
		},
		{
			id:   "vscode-insiders",
			name: "VS Code Insiders",
			clis: []string{"code-insiders"},
			paths: []string{
				under(local, "Programs", "Microsoft VS Code Insiders", "bin", "code-insiders.cmd"),
			},
		},
		{
			id:   "cursor",
			name: "Cursor",
			clis: []string{"cursor"},
			paths: []string{
				under(local, "Programs", "cursor", "resources", "app", "bin", "cursor.cmd"),
				under(programFiles, "Cursor", "resources", "app", "bin", "cursor.cmd"),
			},
		},
		{
			id:   "windsurf",
			name: "Windsurf",
			clis: []string{"windsurf"},
			paths: []string{
				under(local, "Programs", "Windsurf", "bin", "windsurf.cmd"),
			},
		},
		{
			id:   "trae",
			name: "Trae",
			clis: []string{"trae"},
			paths: []string{
				under(local, "Programs", "Trae", "bin", "trae.cmd"),
			},
		},
		{
			id:   "vscodium",
			name: "VSCodium",
			clis: []string{"codium"},
			paths: []string{
				under(local, "Programs", "VSCodium", "bin", "codium.cmd"),
				under(programFiles, "VSCodium", "bin", "codium.cmd"),
			},
		},
		{
			id:   "zed",
			name: "Zed",
			clis: []string{"zed"},
			paths: []string{
				under(local, "Programs", "Zed", "zed.exe"),
			},
		},
	}

	list = append(list, jetbrains(under(local, "JetBrains", "Toolbox", "scripts"), ".cmd")...)
	list = append(list, candidate{
		id:   "sublime",
		name: "Sublime Text",
		clis: []string{"subl"},
		paths: []string{
			under(programFiles, "Sublime Text", "subl.exe"),
		},
	})
	return list
}

// darwinCatalog 列出 macOS 上的常见安装位置。
//
// VS Code 系把 CLI 塞在 app bundle 的 Resources/app/bin 下；JetBrains 的
// 启动器在 Contents/MacOS 下，名字与 PATH 上的 CLI 一致。
func darwinCatalog() []candidate {
	apps := "/Applications"

	return []candidate{
		{
			id:   "vscode",
			name: "VS Code",
			clis: []string{"code"},
			paths: []string{
				under(apps, "Visual Studio Code.app", "Contents", "Resources", "app", "bin", "code"),
			},
		},
		{
			id:   "vscode-insiders",
			name: "VS Code Insiders",
			clis: []string{"code-insiders"},
			paths: []string{
				under(apps, "Visual Studio Code - Insiders.app", "Contents", "Resources", "app", "bin", "code-insiders"),
			},
		},
		{
			id:   "cursor",
			name: "Cursor",
			clis: []string{"cursor"},
			paths: []string{
				under(apps, "Cursor.app", "Contents", "Resources", "app", "bin", "cursor"),
			},
		},
		{
			id:   "windsurf",
			name: "Windsurf",
			clis: []string{"windsurf"},
			paths: []string{
				under(apps, "Windsurf.app", "Contents", "Resources", "app", "bin", "windsurf"),
			},
		},
		{
			id:   "trae",
			name: "Trae",
			clis: []string{"trae"},
			paths: []string{
				under(apps, "Trae.app", "Contents", "Resources", "app", "bin", "trae"),
			},
		},
		{
			id:   "vscodium",
			name: "VSCodium",
			clis: []string{"codium"},
			paths: []string{
				under(apps, "VSCodium.app", "Contents", "Resources", "app", "bin", "codium"),
			},
		},
		{
			id:   "zed",
			name: "Zed",
			clis: []string{"zed"},
			paths: []string{
				under(apps, "Zed.app", "Contents", "MacOS", "cli"),
			},
		},
		{
			id:   "intellij",
			name: "IntelliJ IDEA",
			clis: []string{"idea"},
			paths: []string{
				under(apps, "IntelliJ IDEA.app", "Contents", "MacOS", "idea"),
				under(apps, "IntelliJ IDEA CE.app", "Contents", "MacOS", "idea"),
			},
		},
		{
			id:   "webstorm",
			name: "WebStorm",
			clis: []string{"webstorm"},
			paths: []string{
				under(apps, "WebStorm.app", "Contents", "MacOS", "webstorm"),
			},
		},
		{
			id:   "goland",
			name: "GoLand",
			clis: []string{"goland"},
			paths: []string{
				under(apps, "GoLand.app", "Contents", "MacOS", "goland"),
			},
		},
		{
			id:   "pycharm",
			name: "PyCharm",
			clis: []string{"pycharm"},
			paths: []string{
				under(apps, "PyCharm.app", "Contents", "MacOS", "pycharm"),
				under(apps, "PyCharm CE.app", "Contents", "MacOS", "pycharm"),
			},
		},
		{
			id:   "sublime",
			name: "Sublime Text",
			clis: []string{"subl"},
			paths: []string{
				under(apps, "Sublime Text.app", "Contents", "SharedSupport", "bin", "subl"),
			},
		},
	}
}

// linuxCatalog 列出 Linux 上的常见安装位置。
//
// 发行版多、包管理器多，PATH 是主要来源；这里只补几个 CLI 不一定会进 PATH
// 的发行版路径（snap、flatpak、Toolbox 脚本目录）。
func linuxCatalog() []candidate {
	home, _ := os.UserHomeDir()

	list := []candidate{
		{
			id:   "vscode",
			name: "VS Code",
			clis: []string{"code"},
			paths: []string{
				"/snap/bin/code",
				"/var/lib/flatpak/exports/bin/com.visualstudio.code",
			},
		},
		{
			id:   "vscode-insiders",
			name: "VS Code Insiders",
			clis: []string{"code-insiders"},
			paths: []string{
				"/snap/bin/code-insiders",
			},
		},
		{
			id:   "vscodium",
			name: "VSCodium",
			clis: []string{"codium"},
			paths: []string{
				"/snap/bin/codium",
				"/var/lib/flatpak/exports/bin/com.vscodium.codium",
			},
		},
		{
			id:   "cursor",
			name: "Cursor",
			clis: []string{"cursor"},
		},
		{
			id:   "windsurf",
			name: "Windsurf",
			clis: []string{"windsurf"},
		},
		{
			id:   "trae",
			name: "Trae",
			clis: []string{"trae"},
		},
		{
			id:   "zed",
			name: "Zed",
			clis: []string{"zed"},
		},
		{
			id:   "sublime",
			name: "Sublime Text",
			clis: []string{"subl"},
			paths: []string{
				"/snap/bin/subl",
			},
		},
	}

	return append(list, jetbrains(under(home, ".local", "share", "JetBrains", "Toolbox", "scripts"), "")...)
}
