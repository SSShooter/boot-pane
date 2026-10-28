package editors

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 只在有空白时才加引号：没空格时加了虽然也能解析，但存进配置肉眼看着很怪。
func TestQuotePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\Tools\Code.exe`, `C:\Tools\Code.exe`},
		{`C:\Program Files\Code.exe`, `"C:\Program Files\Code.exe"`},
		{"/opt/my editor/bin/code", `"/opt/my editor/bin/code"`},
	}
	for _, c := range cases {
		if got := quotePath(c.in); got != c.want {
			t.Errorf("quotePath(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// PATH 命中时返回裸命令名：编辑器升级、换安装目录都不会让配置失效。
func TestProbePrefersPath(t *testing.T) {
	dir := t.TempDir()
	name := "bootpane-fake-editor"
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".cmd"
	}
	file := filepath.Join(dir, name+ext)
	if err := os.WriteFile(file, []byte("@echo off\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 安装位置指向一个不存在的文件：优先级反了的话这条会盖住 PATH 的结果。
	c := candidate{
		id:    "fake",
		name:  "Fake",
		clis:  []string{name},
		paths: []string{filepath.Join(dir, "missing.cmd")},
	}
	e, ok := c.probe()
	if !ok {
		t.Fatal("PATH 上有这个命令，应该探测到")
	}
	if e.Command != name || e.Source != SourcePath {
		t.Errorf("应返回裸命令名，得到 %+v", e)
	}
}

// PATH 上没有时退回安装位置；带空格的路径必须加引号，否则存进配置后
// 会被命令行解析拆成「C:\Program」和「Files\...」两段。
func TestProbeFallsBackToInstallPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "My Editor")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "editor.exe")
	if err := os.WriteFile(file, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := candidate{
		id:    "fake",
		name:  "Fake",
		clis:  []string{"bootpane-definitely-missing-cmd"},
		paths: []string{file},
	}
	e, ok := c.probe()
	if !ok {
		t.Fatal("安装位置上确实有文件，应该探测到")
	}
	if want := `"` + file + `"`; e.Command != want {
		t.Errorf("Command = %q，期望 %q", e.Command, want)
	}
	if e.Source != SourceInstall {
		t.Errorf("Source = %q，期望 %q", e.Source, SourceInstall)
	}
}

// 同名目录不算装上：候选路径里有 app bundle 这一级，误判会给出打不开的路径。
func TestProbeIgnoresDirectories(t *testing.T) {
	c := candidate{id: "fake", name: "Fake", paths: []string{t.TempDir()}}
	if _, ok := c.probe(); ok {
		t.Error("目录不应被当成可执行文件")
	}
}

// 清单是手写的，复制粘贴很容易带出重复 id、空名字或相对路径。
func TestCatalogWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range catalog() {
		if c.id == "" || c.name == "" {
			t.Errorf("条目字段不完整: %+v", c)
		}
		if seen[c.id] {
			t.Errorf("重复的 id: %q", c.id)
		}
		seen[c.id] = true

		if len(c.clis) == 0 && len(c.paths) == 0 {
			t.Errorf("%q 既没有命令行也没有安装位置", c.id)
		}
		for _, p := range c.paths {
			if p != "" && !filepath.IsAbs(p) {
				t.Errorf("%q 的路径不是绝对路径: %q", c.id, p)
			}
		}
	}
}

// 探测结果要能直接填进 settings.openWith：有名字、有命令。
func TestDetectResultsAreUsable(t *testing.T) {
	for _, e := range Detect() {
		if e.ID == "" || e.Name == "" || e.Command == "" {
			t.Errorf("结果不完整: %+v", e)
		}
	}
}
