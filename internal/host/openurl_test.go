package host

import "testing"

// 配置来自 JSON，不能让 file:// 或自定义协议借「打开页面」拉起任意程序。
func TestOpenExternalURLRejectsNonHTTP(t *testing.T) {
	for _, raw := range []string{
		"",
		"localhost:5173",
		"file:///C:/Windows/System32/calc.exe",
		"javascript:alert(1)",
		"http://",
	} {
		if err := OpenExternalURL(raw); err == nil {
			t.Errorf("%q 应该被拒绝", raw)
		}
	}
}

// URL 必须整条作为一个参数传出去：带 & 的地址交给 shell 会被截断。
func TestBrowserCommandPassesURLAsOneArg(t *testing.T) {
	raw := "http://localhost:5173/a?b=1&c=2"
	bin, args := browserCommand(raw)

	if bin == "" {
		t.Fatal("浏览器命令不能为空")
	}
	found := false
	for _, arg := range args {
		if arg == raw {
			found = true
		}
	}
	if !found {
		t.Fatalf("URL 应作为独立参数传入，得到 %q %v", bin, args)
	}
}
