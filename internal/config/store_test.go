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

const repoBody = `{
	"version": 1,
	"project": {"name": "仓库里的名字", "tags": ["go"]},
	"tasks": [
		{"id": "web", "label": "仓库的前端", "command": "pnpm dev"},
		{"id": "api", "label": "启动 API", "command": "go run ./cmd/api"}
	]
}`

func TestResolveUsesGlobalByDefault(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, repoBody)

	s := newStore(t)
	rp := s.Resolve(Project{
		ID:    "p1",
		Name:  "我的项目",
		Path:  repo,
		Tasks: []Task{{ID: "only", Label: "只有全局的", Command: "echo hi"}},
	})

	if rp.ConfigSource != SourceGlobal {
		t.Errorf("默认应使用全局配置，得到 %q", rp.ConfigSource)
	}
	if len(rp.Tasks) != 1 || rp.Tasks[0].ID != "only" {
		t.Fatalf("默认不该读仓库配置，得到 %+v", rp.Tasks)
	}
	// 仓库里确实有文件这件事仍要告诉界面，供用户决定是否切换
	if !rp.HasRepoCfg {
		t.Error("应探测到仓库里存在 .bootpane.json")
	}
	if rp.Name != "我的项目" {
		t.Errorf("名称不应被仓库配置覆盖，得到 %q", rp.Name)
	}
}

func TestResolveUsesRepoWhenEnabled(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, repoBody)

	s := newStore(t)
	rp := s.Resolve(Project{
		ID:            "p1",
		Path:          repo,
		UseRepoConfig: true,
		Tasks:         []Task{{ID: "only", Label: "全局任务", Command: "echo hi"}},
	})

	if rp.ConfigSource != SourceRepo {
		t.Errorf("应使用项目内配置，得到 %q", rp.ConfigSource)
	}
	if len(rp.Tasks) != 2 {
		t.Fatalf("开启后应完全以仓库为准，得到 %+v", rp.Tasks)
	}
	for _, task := range rp.Tasks {
		if task.ID == "only" {
			t.Error("全局任务不应出现在项目内配置模式里")
		}
	}
	// 名称没填时用仓库里的元信息兜底
	if rp.Name != "仓库里的名字" {
		t.Errorf("名称应回落到仓库配置，得到 %q", rp.Name)
	}
	if len(rp.Tags) != 1 || rp.Tags[0] != "go" {
		t.Errorf("标签应合并仓库标签，得到 %v", rp.Tags)
	}
}

func TestResolveErrorsWhenRepoConfigMissing(t *testing.T) {
	repo := t.TempDir()
	s := newStore(t)
	rp := s.Resolve(Project{ID: "p1", Path: repo, UseRepoConfig: true})

	if rp.HasRepoCfg {
		t.Error("仓库里没有配置文件")
	}
	if !hasIssue(rp.Issues, "error", "没有 "+RepoConfigName) {
		t.Fatalf("开启项目内配置但文件不存在时应报错，得到 %+v", rp.Issues)
	}
}

func TestSetUseRepoConfigPersists(t *testing.T) {
	repo := t.TempDir()
	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Name: "n", Path: repo}}

	if err := s.SetUseRepoConfig("p1", true); err != nil {
		t.Fatal(err)
	}
	if !s.ws.Projects[0].UseRepoConfig {
		t.Error("开关没有写回内存中的注册表")
	}

	// 重新从磁盘加载，确认落盘了
	reloaded := &Store{dir: s.dir}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if !reloaded.ws.Projects[0].UseRepoConfig {
		t.Error("开关没有落盘")
	}
}

func TestParseProjectFileToleratesFences(t *testing.T) {
	raw := "这是配置：\n```json\n" + repoBody + "\n```\n希望有帮助！"
	pf, err := ParseProjectFile(raw)
	if err != nil {
		t.Fatalf("应能容忍代码围栏和前后文字: %v", err)
	}
	if len(pf.Tasks) != 2 {
		t.Errorf("应解析出 2 个任务，得到 %d", len(pf.Tasks))
	}
}

func TestParseProjectFileRejectsEmpty(t *testing.T) {
	if _, err := ParseProjectFile("模型没有给出 JSON"); err == nil {
		t.Error("没有 JSON 时应当报错")
	}
	if _, err := ParseProjectFile(`{"version":1,"tasks":[]}`); err == nil {
		t.Error("空任务列表应当报错")
	}
}

func TestBuildPromptMentionsProjectPath(t *testing.T) {
	prompt := BuildPrompt(ResolvedProject{Path: `D:\code\my-shop`})
	if !strings.Contains(prompt, `D:\code\my-shop`) {
		t.Error("prompt 里必须带上项目路径，否则用户不知道该去哪用")
	}
	if !strings.Contains(prompt, "waitFor") {
		t.Error("prompt 里应说明 waitFor 字段")
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
	if issues := Validate(&rp); !hasIssue(issues, "error", "依赖的任务") {
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
	if issues := Validate(&rp); !hasIssue(issues, "error", "依赖成环") {
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
