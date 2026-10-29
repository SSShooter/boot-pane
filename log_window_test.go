package main

import (
	"net/url"
	"testing"
)

// 面板快照经 URL 转一圈必须原样回来。
// 项目 id 和任务 id 都是路径形式，带空格、#、& 的情况真实存在（目录名里就有），
// 少转义一个字符，独立窗口拿到的就是被截断的半份 JSON。
func TestLogWindowURLRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		state string
	}{
		{"普通", `{"source":"ab12","key":"p/t","tabs":[{"key":"p/t","projectId":"p","taskId":"t"}]}`},
		{"含空格和井号", `{"source":"ab12","key":"my shop/web#1","tabs":[{"key":"my shop/web#1","projectId":"my shop","taskId":"web#1"}]}`},
		{"含分隔符", `{"source":"ab12","key":"a&b/c=d?e","tabs":[{"key":"a&b/c=d?e","projectId":"a&b","taskId":"c=d?e"}]}`},
		{"中文", `{"source":"ab12","key":"项目/启动网页端","tabs":[{"key":"项目/启动网页端","projectId":"项目","taskId":"启动网页端"}]}`},
		{"空标签", `{"source":"ab12","key":"","tabs":[]}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(logWindowURL(c.state))
			if err != nil {
				t.Fatalf("解析地址失败: %v", err)
			}
			if u.Path != "/" {
				t.Fatalf("path = %q，期望 /", u.Path)
			}
			if got := u.Query().Get(logWindowQuery); got != c.state {
				t.Fatalf("回读状态 = %q，期望 %q", got, c.state)
			}
		})
	}
}

// 参数名是前后端约定的一部分，改了这里必须同时改前端，否则独立窗口读不到状态。
func TestLogWindowQueryName(t *testing.T) {
	if logWindowQuery != "log" {
		t.Fatalf("logWindowQuery = %q，前端 LOG_WINDOW_QUERY 同步了吗", logWindowQuery)
	}
}

// 空状态没有任何意义：没有标签就没有可弹的面板，与其开一个空窗口不如报错。
func TestLogWindowOpenRejectsEmptyState(t *testing.T) {
	svc := NewLogWindowService()
	for _, state := range []string{"", "   ", "\n\t"} {
		if err := svc.Open(state); err == nil {
			t.Fatalf("state = %q 时应拒绝打开窗口", state)
		}
	}
}
