package config

import "testing"

// 显式地址优先，且要顺手去掉首尾空格——配置文件里手写的值经常带空格。
func TestTaskPageURLExplicitWins(t *testing.T) {
	task := Task{
		ID:      "web",
		Group:   "前端",
		OpenURL: " https://shop.example.com/admin ",
		WaitFor: &WaitFor{Type: WaitPort, Port: 5173},
	}
	if got := task.PageURL(); got != "https://shop.example.com/admin" {
		t.Errorf("应优先使用显式 openUrl，得到 %q", got)
	}
}

// 没配 openUrl 时，前端类任务按端口兜底，省得用户为最常见的场景多写一行。
func TestTaskPageURLDerivesFromPortForFrontend(t *testing.T) {
	cases := []Task{
		{ID: "web", WaitFor: &WaitFor{Type: WaitPort, Port: 5173}},
		{ID: "h5", Group: "H5", WaitFor: &WaitFor{Type: WaitPort, Port: 5173}},
		{ID: "front", Label: "启动网页端", WaitFor: &WaitFor{Type: WaitPort, Port: 5173}},
	}
	for _, task := range cases {
		if got := task.PageURL(); got != "http://localhost:5173" {
			t.Errorf("%q 应兜底推导出页面地址，得到 %q", task.ID, got)
		}
	}
}

// 后端也监听端口，但不能因为「有端口」就挂一个打开按钮。
func TestTaskPageURLSkipsBackendTasks(t *testing.T) {
	cases := []Task{
		{
			ID:      "api",
			Label:   "启动后端",
			Group:   "后端",
			Command: "go run ./cmd/web",
			WaitFor: &WaitFor{Type: WaitPort, Port: 8080},
		},
		{
			// 名字里带 web，但返回 JSON 的接口不该有页面按钮。
			ID:      "web-api",
			Label:   "启动 Web API",
			WaitFor: &WaitFor{Type: WaitPort, Port: 8080},
		},
	}
	for _, task := range cases {
		if got := task.PageURL(); got != "" {
			t.Errorf("%q 是后端任务，不该有页面地址，得到 %q", task.ID, got)
		}
	}
}

// 只等日志的任务没有端口可用，除非显式写了 openUrl。
func TestTaskPageURLRequiresPort(t *testing.T) {
	task := Task{
		ID:      "web",
		Group:   "前端",
		WaitFor: &WaitFor{Type: WaitLog, Log: "ready in"},
	}
	if got := task.PageURL(); got != "" {
		t.Errorf("没有端口时不该推导地址，得到 %q", got)
	}
}

// 显式地址写错时不能悄悄退回端口推导：用户改了一行配置却看不到任何变化，
// 只会以为这个字段没生效。
func TestTaskPageURLRejectsInvalidExplicit(t *testing.T) {
	task := Task{
		ID:      "web",
		Group:   "前端",
		OpenURL: "htp://localhost:5173",
		WaitFor: &WaitFor{Type: WaitPort, Port: 5173},
	}
	if got := task.PageURL(); got != "" {
		t.Errorf("非法地址不该生成页面地址，得到 %q", got)
	}
}

func TestIsHTTPURL(t *testing.T) {
	ok := []string{"http://localhost:5173", "https://example.com/a?b=1", " HTTP://127.0.0.1:8080 "}
	for _, raw := range ok {
		if !IsHTTPURL(raw) {
			t.Errorf("%q 应判定为合法地址", raw)
		}
	}

	bad := []string{"", "localhost:5173", "file:///C:/Windows/System32/calc.exe", "javascript:alert(1)", "http://"}
	for _, raw := range bad {
		if IsHTTPURL(raw) {
			t.Errorf("%q 不该判定为合法地址", raw)
		}
	}
}

func TestPageURLsMap(t *testing.T) {
	got := pageURLs([]Task{
		{ID: "web", Label: "启动网页端", WaitFor: &WaitFor{Type: WaitPort, Port: 5173}},
		{ID: "api", Label: "启动 API", WaitFor: &WaitFor{Type: WaitPort, Port: 8080}},
	})
	if len(got) != 1 || got["web"] != "http://localhost:5173" {
		t.Errorf("只应包含前端任务，得到 %+v", got)
	}

	if pageURLs([]Task{{ID: "api", Command: "go run ./cmd/api"}}) != nil {
		t.Error("没有任何页面时应返回 nil，而不是空 map")
	}
}

func TestValidateWarnsInvalidOpenURL(t *testing.T) {
	rp := ResolvedProject{
		Exists: true,
		Path:   t.TempDir(),
		Tasks: []Task{{
			ID:      "web",
			Command: "pnpm dev",
			OpenURL: "file:///C:/Windows/System32/calc.exe",
		}},
	}
	issues := Validate(&rp)
	if !hasIssue(issues, "warn", "openUrl") {
		t.Fatalf("应提示 openUrl 不合法，得到 %+v", issues)
	}
	// 只是提示：地址写错不该拦住启动。
	for _, is := range issues {
		if is.Level == "error" {
			t.Errorf("openUrl 不合法不该是 error，得到 %+v", is)
		}
	}
}

// 解析后的项目视图要带上页面地址表，前端靠它决定显示哪些按钮。
func TestResolveFillsPageURLs(t *testing.T) {
	s := newStore(t)
	rp := s.Resolve(Project{
		ID:   "p1",
		Path: t.TempDir(),
		Tasks: []Task{
			{ID: "web", Label: "启动网页端", Command: "pnpm dev", WaitFor: &WaitFor{Type: WaitPort, Port: 5173}},
			{ID: "api", Label: "启动 API", Command: "go run ./cmd/api", WaitFor: &WaitFor{Type: WaitPort, Port: 8080}},
		},
	})

	if rp.PageURLs["web"] != "http://localhost:5173" {
		t.Errorf("应解析出前端任务的地址，得到 %+v", rp.PageURLs)
	}
	if _, ok := rp.PageURLs["api"]; ok {
		t.Errorf("后端任务不该出现在页面地址表里，得到 %+v", rp.PageURLs)
	}
}
