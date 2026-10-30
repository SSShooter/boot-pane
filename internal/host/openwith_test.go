package host

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommandLine(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"code", []string{"code"}},
		{"code  {path}", []string{"code", "{path}"}},
		{`"C:\Program Files\Microsoft VS Code\Code.exe" -n {path}`,
			[]string{`C:\Program Files\Microsoft VS Code\Code.exe`, "-n", "{path}"}},
		{`code "" {path}`, []string{"code", "", "{path}"}},
		{"  ", nil},
	}

	for _, c := range cases {
		if got := splitCommandLine(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCommandLine(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// 不写占位符时项目路径追加到末尾——`code`、`cursor` 这些 CLI 的约定。
func TestResolveOpenWithAppendsPath(t *testing.T) {
	bin, args, err := resolveOpenWith("go", `C:\my project`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(bin), "go.exe") && !strings.HasSuffix(strings.ToLower(bin), "go") {
		t.Errorf("应定位到 go 可执行文件，得到 %q", bin)
	}
	if want := []string{`C:\my project`}; !reflect.DeepEqual(args, want) {
		t.Errorf("参数 = %q，期望 %q", args, want)
	}
}

func TestResolveOpenWithSubstitutesPlaceholder(t *testing.T) {
	_, args, err := resolveOpenWith(`go --new-window "{path}" -n`, "/repo")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	want := []string{"--new-window", "/repo", "-n"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("参数 = %q，期望 %q", args, want)
	}
}

// 拼错的命令要在保存设置时就报错，而不是等到点「打开目录」。
func TestLookupOpenWithRejectsUnknownCommand(t *testing.T) {
	if _, err := LookupOpenWith("definitely-not-installed-cmd"); err == nil {
		t.Fatal("不存在的命令应返回错误")
	}
	if _, err := LookupOpenWith("   "); err == nil {
		t.Fatal("空命令应返回错误")
	}
}
