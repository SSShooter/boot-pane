package editors

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// shimExt 是当前平台上 VS Code 家族 CLI 垫片的后缀。
func shimExt() string {
	if runtime.GOOS == "windows" {
		return ".cmd"
	}
	return ""
}

// writeForkShim 造一个垫片文件；shim 为 true 时带上真实垫片的内容特征。
func writeForkShim(t *testing.T, path string, shim bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "@echo off\n"
	if shim {
		body = "@echo off\nset ELECTRON_RUN_AS_NODE=1\n" +
			`"%~dp0..\X.exe" "%~dp0..\resources\app\out\cli.js" %*` + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// 清单里没列举到的分支编辑器（Kiro、Antigravity 这类）也要能扫到，
// 展示名用安装目录名，而不是垫片文件名。
func TestScanCodeForksFindsUnlistedFork(t *testing.T) {
	root := t.TempDir()
	shim := filepath.Join(root, "Kiro", "bin", "kiro"+shimExt())
	writeForkShim(t, shim, true)

	list := scanCodeForks([]string{root})
	if len(list) != 1 {
		t.Fatalf("应扫到 1 个编辑器，得到 %d 个: %+v", len(list), list)
	}
	e := list[0]
	if e.Name != "Kiro" {
		t.Errorf("Name = %q，期望用安装目录名 Kiro", e.Name)
	}
	if e.Command != quotePath(shim) {
		t.Errorf("Command = %q，期望 %q", e.Command, quotePath(shim))
	}
	if e.Source != SourceScan {
		t.Errorf("Source = %q，期望 %q", e.Source, SourceScan)
	}
}

// Cursor 那一支的垫片放在 resources/app/bin 下，布局不同但指纹一致。
func TestScanCodeForksHandlesResourcesLayout(t *testing.T) {
	root := t.TempDir()
	var shim string
	if runtime.GOOS == "darwin" {
		shim = filepath.Join(root, "Cursor.app", "Contents", "Resources", "app", "bin", "cursor")
	} else {
		shim = filepath.Join(root, "Cursor", "resources", "app", "bin", "cursor"+shimExt())
	}
	writeForkShim(t, shim, true)

	list := scanCodeForks([]string{root})
	if len(list) != 1 {
		t.Fatalf("应扫到 1 个编辑器，得到 %d 个: %+v", len(list), list)
	}
	if list[0].Name != "Cursor" {
		t.Errorf("Name = %q，期望 Cursor", list[0].Name)
	}
}

// 没有指纹的 .cmd 只是普通批处理，不能当成编辑器列出来。
func TestScanCodeForksIgnoresForeignShim(t *testing.T) {
	root := t.TempDir()
	writeForkShim(t, filepath.Join(root, "SomeApp", "bin", "someapp"+shimExt()), false)

	if got := scanCodeForks([]string{root}); len(got) != 0 {
		t.Errorf("不该识别无关的批处理，得到 %+v", got)
	}
}

// 同一个程序不能因为「PATH 命中」和「清单命中」各算一次。
func TestDetectHasNoDuplicateCommands(t *testing.T) {
	seen := make(map[string]bool)
	for _, e := range Detect() {
		if seen[e.Command] {
			t.Errorf("重复的命令: %q", e.Command)
		}
		seen[e.Command] = true
	}
}
