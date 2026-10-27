package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStore 构造一个只用于测试的 Store，不触碰真实用户配置目录。
func newStore(t *testing.T) *Store {
	t.Helper()
	return &Store{dir: t.TempDir(), ws: Workspace{Version: 1}}
}

func writeRepoConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, RepoConfigName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRepoConfigOverridesLocal(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, `{
		"version": 1,
		"project": {"name": "团队名", "tags": ["go"]},
		"tasks": [
			{"id": "web", "label": "团队的前端", "command": "pnpm dev"},
			{"id": "api", "label": "启动 API", "command": "go run ./cmd/api"}
		]
	}`)

	s := newStore(t)
	rp := s.Resolve(Project{
		ID:   "p1",
		Name: "本地名",
		Path: repo,
		Tags: []string{"local-tag"},
		LocalTasks: []Task{
			{ID: "web", Label: "我的前端", Command: "pnpm dev --port 9999"},
			{ID: "debug", Label: "调试", Command: "dlv debug"},
		},
	})

	if rp.Name != "本地名" {
		t.Errorf("注册表里的名字应优先，得到 %q", rp.Name)
	}
	if !rp.HasRepoCfg {
		t.Error("应识别到仓库配置存在")
	}
	if len(rp.Tasks) != 3 {
		t.Fatalf("合并后应有 3 个任务，得到 %d", len(rp.Tasks))
	}

	byID := map[string]Task{}
	for _, task := range rp.Tasks {
		byID[task.ID] = task
	}
	if got := byID["web"]; got.Command != "pnpm dev" || got.Origin != OriginRepo {
		t.Errorf("同 id 时仓库配置应胜出，得到 command=%q origin=%q", got.Command, got.Origin)
	}
	if got := byID["debug"]; got.Origin != OriginLocal {
		t.Errorf("个人层任务应标记为 local，得到 %q", got.Origin)
	}
	if len(rp.Tags) != 2 {
		t.Errorf("标签应合并去重，得到 %v", rp.Tags)
	}
}

func TestValidateMissingDependency(t *testing.T) {
	rp := ResolvedProject{
		Exists: true,
		Path:   t.TempDir(),
		Tasks: []Task{
			{ID: "web", Command: "pnpm dev", DependsOn: []string{"nope"}},
		},
	}
	issues := Validate(&rp)
	if !hasIssue(issues, "error", "依赖的任务") {
		t.Fatalf("应报依赖不存在，得到 %+v", issues)
	}
}

func TestValidateCycle(t *testing.T) {
	rp := ResolvedProject{
		Exists: true,
		Path:   t.TempDir(),
		Tasks: []Task{
			{ID: "a", Command: "echo a", DependsOn: []string{"b"}},
			{ID: "b", Command: "echo b", DependsOn: []string{"a"}},
		},
	}
	issues := Validate(&rp)
	if !hasIssue(issues, "error", "依赖成环") {
		t.Fatalf("应检出依赖环，得到 %+v", issues)
	}
}

func TestValidatePortConflictIsWarning(t *testing.T) {
	rp := ResolvedProject{
		Exists: true,
		Path:   t.TempDir(),
		Tasks: []Task{
			{ID: "a", Command: "echo a", WaitFor: &WaitFor{Type: WaitPort, Port: 5173}},
			{ID: "b", Command: "echo b", WaitFor: &WaitFor{Type: WaitPort, Port: 5173}},
		},
	}
	issues := Validate(&rp)
	if !hasIssue(issues, "warn", "端口 5173") {
		t.Fatalf("端口重复应给出 warn，得到 %+v", issues)
	}
	if hasIssue(issues, "error", "端口") {
		t.Error("端口重复不应阻断启动")
	}
}

func TestValidateMissingCommand(t *testing.T) {
	rp := ResolvedProject{
		Exists: true,
		Path:   t.TempDir(),
		Tasks:  []Task{{ID: "empty", Label: "空命令"}},
	}
	if issues := Validate(&rp); !hasIssue(issues, "error", "未配置启动命令") {
		t.Fatalf("空命令应报错，得到 %+v", issues)
	}
}

func TestValidateSkipsWhenDirMissing(t *testing.T) {
	rp := ResolvedProject{Exists: false, Path: filepath.Join(t.TempDir(), "gone")}
	if issues := Validate(&rp); len(issues) != 0 {
		t.Errorf("目录不存在时不该逐条校验任务，得到 %+v", issues)
	}
}

func hasIssue(issues []Issue, level, contains string) bool {
	for _, is := range issues {
		if is.Level == level && strings.Contains(is.Message, contains) {
			return true
		}
	}
	return false
}
