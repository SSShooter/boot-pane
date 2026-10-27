package config

import (
	"encoding/json"
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

// 星标必须落在全局注册表里，重启应用后还在。
func TestSetStarPersists(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{
		ID:    "p1",
		Path:  t.TempDir(),
		Tasks: []Task{{ID: "web", Label: "前端", Command: "pnpm dev"}},
	}}

	if err := s.SetStar("p1", "web", true); err != nil {
		t.Fatal(err)
	}

	rp, err := s.Get("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.StarredTaskIDs) != 1 || rp.StarredTaskIDs[0] != "web" {
		t.Fatalf("视图里应带上星标 id，得到 %v", rp.StarredTaskIDs)
	}

	reloaded := &Store{dir: s.dir}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ws.Stars) != 1 {
		t.Fatalf("星标没有落盘，得到 %+v", reloaded.ws.Stars)
	}
	if got := reloaded.ws.Stars[0]; got.ProjectID != "p1" || got.TaskID != "web" {
		t.Errorf("落盘的星标内容不对：%+v", got)
	}
}

// 重复设置同一个状态不该堆出多条记录，否则列表里会出现重复卡片。
func TestSetStarIsIdempotent(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Path: t.TempDir()}}

	for i := 0; i < 3; i++ {
		if err := s.SetStar("p1", "web", true); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.ws.Stars) != 1 {
		t.Fatalf("重复星标应只有一条记录，得到 %d 条", len(s.ws.Stars))
	}

	for i := 0; i < 3; i++ {
		if err := s.SetStar("p1", "web", false); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.ws.Stars) != 0 {
		t.Fatalf("取消星标后应为空，得到 %+v", s.ws.Stars)
	}
}

func TestSetStarRejectsUnknownProject(t *testing.T) {
	s := newStore(t)
	if err := s.SetStar("nope", "web", true); err == nil {
		t.Error("项目不存在时应当报错")
	}
	if len(s.ws.Stars) != 0 {
		t.Errorf("失败的调用不该留下星标，得到 %+v", s.ws.Stars)
	}
}

// 移除项目要连带清掉它的星标：重新添加会生成同一个 ID，
// 不清就会让旧星标诈尸冒出来。
func TestRemoveDropsStars(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{
		{ID: "p1", Path: t.TempDir()},
		{ID: "p2", Path: t.TempDir()},
	}
	if err := s.SetStar("p1", "web", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStar("p2", "api", true); err != nil {
		t.Fatal(err)
	}

	if err := s.Remove("p1"); err != nil {
		t.Fatal(err)
	}
	if len(s.ws.Stars) != 1 || s.ws.Stars[0].ProjectID != "p2" {
		t.Fatalf("只应清掉 p1 的星标，得到 %+v", s.ws.Stars)
	}
}

// 项目内配置模式下任务来自仓库文件，星标照样要能设——
// 这正是星标不塞进 Task 的原因。
func TestSetStarWorksInRepoConfigMode(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, repoBody)

	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Path: repo, UseRepoConfig: true}}

	if err := s.SetStar("p1", "api", true); err != nil {
		t.Fatal(err)
	}

	rp, err := s.Get("p1")
	if err != nil {
		t.Fatal(err)
	}
	if rp.ConfigSource != SourceRepo {
		t.Fatalf("前提是项目内配置模式，得到 %q", rp.ConfigSource)
	}
	if len(rp.StarredTaskIDs) != 1 || rp.StarredTaskIDs[0] != "api" {
		t.Fatalf("仓库里的任务也应能星标，得到 %v", rp.StarredTaskIDs)
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

func TestSettingsDefaultToOff(t *testing.T) {
	// 老版本写出来的 workspace.json 里没有 settings 字段，
	// 读进来必须落在「不自动打开日志」上，不能因为零值之外的默认值而改变行为。
	dir := t.TempDir()
	old := `{"version":1,"projects":[],"stars":[]}`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Store{dir: dir}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if s.Settings().AutoOpenLogOnStart {
		t.Error("旧配置文件应读成不自动打开日志")
	}
}

func TestSetAutoOpenLogOnStartPersists(t *testing.T) {
	s := newStore(t)

	if err := s.SetAutoOpenLogOnStart(true); err != nil {
		t.Fatal(err)
	}
	if !s.Settings().AutoOpenLogOnStart {
		t.Error("开关没有写回内存")
	}

	// 重新从磁盘加载，确认落盘
	reloaded := &Store{dir: s.dir}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if !reloaded.Settings().AutoOpenLogOnStart {
		t.Error("开关没有落盘")
	}

	if err := s.SetAutoOpenLogOnStart(false); err != nil {
		t.Fatal(err)
	}
	if s.Settings().AutoOpenLogOnStart {
		t.Error("关闭开关没有生效")
	}
}

// 偏好存在注册表顶层，和项目、星标同级，不跟着仓库配置走。
func TestSettingsSurviveRepoConfigMode(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, repoBody)
	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Name: "n", Path: repo, UseRepoConfig: true}}

	if err := s.SetAutoOpenLogOnStart(true); err != nil {
		t.Fatal(err)
	}
	// 读仓库配置不应该碰到偏好字段
	_ = s.Projects()
	if !s.Settings().AutoOpenLogOnStart {
		t.Error("解析项目时不该影响本机偏好")
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

// 编辑框和导入框的区别就在这里：把启动项全删光再保存是正常操作，不能拦。
func TestParseEditableConfigAllowsEmpty(t *testing.T) {
	pf, err := ParseEditableConfig(`{"version":1,"tasks":[]}`)
	if err != nil {
		t.Fatalf("编辑框应允许空任务列表: %v", err)
	}
	if len(pf.Tasks) != 0 {
		t.Errorf("应解析出 0 个任务，得到 %d", len(pf.Tasks))
	}
}

// 编辑框里看到的就是「当前真正生效的配置」，两种来源都要能拿到。
func TestConfigSnapshotGlobal(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{
		ID:    "p1",
		Name:  "我的项目",
		Path:  t.TempDir(),
		Tags:  []string{"go"},
		Tasks: []Task{{ID: "api", Label: "启动 API", Command: "go run ./cmd/api"}},
	}}

	raw, err := s.ConfigSnapshot("p1")
	if err != nil {
		t.Fatal(err)
	}
	var pf ProjectFile
	if err := json.Unmarshal([]byte(raw), &pf); err != nil {
		t.Fatalf("快照应当是合法 JSON: %v\n%s", err, raw)
	}
	if pf.Project.Name != "我的项目" {
		t.Errorf("应带上项目名，得到 %q", pf.Project.Name)
	}
	if len(pf.Tasks) != 1 || pf.Tasks[0].ID != "api" {
		t.Errorf("应带上注册表里的任务，得到 %+v", pf.Tasks)
	}
}

// 空任务列表要序列化成 []，不能是 null——编辑框里看到 null 会让人以为配置坏了。
func TestConfigSnapshotEmptyTasksIsArray(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Path: t.TempDir()}}

	raw, err := s.ConfigSnapshot("p1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"tasks": []`) {
		t.Errorf("空任务应输出 []，得到：\n%s", raw)
	}
}

// 项目内配置模式返回仓库文件原文，不能先反序列化再序列化把用户的排版搅乱。
func TestConfigSnapshotRepoReturnsRaw(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, repoBody)

	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Path: repo, UseRepoConfig: true}}

	raw, err := s.ConfigSnapshot("p1")
	if err != nil {
		t.Fatal(err)
	}
	if raw != repoBody {
		t.Errorf("应原样返回仓库文件内容，得到：\n%s", raw)
	}
}

// 切到项目内配置但仓库里还没建文件时，也要能给出一个可编辑的骨架。
func TestConfigSnapshotRepoWithoutFile(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Name: "空项目", Path: t.TempDir(), UseRepoConfig: true}}

	raw, err := s.ConfigSnapshot("p1")
	if err != nil {
		t.Fatal(err)
	}
	var pf ProjectFile
	if err := json.Unmarshal([]byte(raw), &pf); err != nil {
		t.Fatalf("骨架应当是合法 JSON: %v\n%s", err, raw)
	}
	if pf.Version != 1 {
		t.Errorf("骨架应带 version 1，得到 %d", pf.Version)
	}
}

// 全局模式保存：任务写回注册表并落盘。
func TestApplyConfigGlobalPersists(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{
		ID:    "p1",
		Name:  "旧名字",
		Path:  t.TempDir(),
		Tasks: []Task{{ID: "old", Label: "旧任务", Command: "echo old"}},
	}}

	pf := ProjectFile{
		Version: 1,
		Project: Meta{Name: "新名字", Tags: []string{"go"}},
		Tasks:   []Task{{ID: "web", Label: "前端", Command: "pnpm dev"}},
	}
	if err := s.ApplyConfig("p1", pf, true); err != nil {
		t.Fatal(err)
	}

	reloaded := &Store{dir: s.dir}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	got := reloaded.ws.Projects[0]
	if len(got.Tasks) != 1 || got.Tasks[0].ID != "web" {
		t.Fatalf("任务没有写回注册表，得到 %+v", got.Tasks)
	}
	if got.Name != "新名字" {
		t.Errorf("syncMeta 为 true 时应同步项目名，得到 %q", got.Name)
	}
}

// 导入要把 JSON 里的名称和描述写进项目：模型给了却不用，界面上就看不到。
// 标签不在 prompt 的输出约定里，导入时保持原值。
func TestApplyConfigImportUsesNameAndDescription(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{
		ID:   "p1",
		Name: "用户起的名字",
		Path: t.TempDir(),
		Tags: []string{"keep"},
	}}

	pf := ProjectFile{
		Version: 1,
		Project: Meta{Name: "my-shop", Description: "uni-app 商城，前端 h5 + 后端 Go API", Tags: []string{"other"}},
		Tasks:   []Task{{ID: "web", Command: "pnpm dev"}},
	}
	if err := s.ApplyConfig("p1", pf, false); err != nil {
		t.Fatal(err)
	}

	got := s.ws.Projects[0]
	if got.Name != "my-shop" {
		t.Errorf("导入应使用 JSON 里的名称，得到 %q", got.Name)
	}
	if got.Description != "uni-app 商城，前端 h5 + 后端 Go API" {
		t.Errorf("导入应使用 JSON 里的描述，得到 %q", got.Description)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "keep" {
		t.Errorf("导入不该改标签，得到 %v", got.Tags)
	}
}

// 模型省略字段是允许的（prompt 明确说拿不准就省略），此时不能清空已有值。
func TestApplyConfigImportEmptyMetaKeepsExisting(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{
		ID:          "p1",
		Name:        "my-shop",
		Description: "我自己写的描述",
		Path:        t.TempDir(),
	}}

	pf := ProjectFile{
		Version: 1,
		Project: Meta{Name: "  "},
		Tasks:   []Task{{ID: "web", Command: "pnpm dev"}},
	}
	if err := s.ApplyConfig("p1", pf, false); err != nil {
		t.Fatal(err)
	}

	got := s.ws.Projects[0]
	if got.Name != "my-shop" {
		t.Errorf("导入没给名称时不该清空，得到 %q", got.Name)
	}
	if got.Description != "我自己写的描述" {
		t.Errorf("导入没给描述时不该清空，得到 %q", got.Description)
	}
}

// 编辑框保存是另一套规则：框里写了什么就是什么，包括清空。
// 否则编辑框里把描述删掉再保存，界面上还留着旧值，框就是「假的」。
func TestApplyConfigEditSyncsDescription(t *testing.T) {
	s := newStore(t)
	s.ws.Projects = []Project{{
		ID:          "p1",
		Name:        "my-shop",
		Description: "旧描述",
		Path:        t.TempDir(),
	}}

	pf := ProjectFile{Version: 1, Project: Meta{Description: "  新描述  "}}
	if err := s.ApplyConfig("p1", pf, true); err != nil {
		t.Fatal(err)
	}
	if got := s.ws.Projects[0].Description; got != "新描述" {
		t.Errorf("编辑保存应同步描述并去掉首尾空白，得到 %q", got)
	}

	// 再保存一次空描述：允许清空
	if err := s.ApplyConfig("p1", ProjectFile{Version: 1}, true); err != nil {
		t.Fatal(err)
	}
	if got := s.ws.Projects[0].Description; got != "" {
		t.Errorf("编辑保存应允许清空描述，得到 %q", got)
	}
}

// 描述和名字走同一条回退链：注册表优先，空了才看仓库 meta。
func TestResolveDescriptionFallback(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, `{
		"version": 1,
		"project": {"description": "仓库里的描述"},
		"tasks": [{"id": "web", "command": "pnpm dev"}]
	}`)

	s := newStore(t)

	rp := s.Resolve(Project{ID: "p1", Name: "x", Path: repo})
	if rp.Description != "仓库里的描述" {
		t.Errorf("注册表没描述时应回落到仓库 meta，得到 %q", rp.Description)
	}

	rp = s.Resolve(Project{ID: "p1", Name: "x", Description: "注册表的描述", Path: repo})
	if rp.Description != "注册表的描述" {
		t.Errorf("注册表有描述时应优先，得到 %q", rp.Description)
	}
}

// 项目内配置模式保存：直接改仓库里的 .bootpane.json。
func TestApplyConfigRepoWritesFile(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, repoBody)

	s := newStore(t)
	s.ws.Projects = []Project{{ID: "p1", Path: repo, UseRepoConfig: true}}

	pf := ProjectFile{
		Version: 1,
		Tasks:   []Task{{ID: "only", Label: "只剩一个", Command: "echo hi"}},
	}
	if err := s.ApplyConfig("p1", pf, true); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(repo, RepoConfigName))
	if err != nil {
		t.Fatal(err)
	}
	var got ProjectFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("写入的文件应是合法 JSON: %v", err)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].ID != "only" {
		t.Errorf("仓库文件没有更新，得到 %+v", got.Tasks)
	}
	// 临时文件不能留在用户的工作区里
	if _, err := os.Stat(filepath.Join(repo, RepoConfigName+".tmp")); err == nil {
		t.Error("原子写入的临时文件应当被重命名掉，不能残留")
	}
}

// 项目内配置模式下，写的是仓库文件而不是注册表里的任务。
func TestApplyConfigRepoLeavesRegistryAlone(t *testing.T) {
	repo := t.TempDir()
	writeRepoConfig(t, repo, repoBody)

	s := newStore(t)
	s.ws.Projects = []Project{{
		ID:            "p1",
		Path:          repo,
		UseRepoConfig: true,
		Tasks:         []Task{{ID: "global-only", Command: "echo global"}},
	}}

	pf := ProjectFile{Version: 1, Tasks: []Task{{ID: "repo-only", Command: "echo repo"}}}
	if err := s.ApplyConfig("p1", pf, true); err != nil {
		t.Fatal(err)
	}

	if len(s.ws.Projects[0].Tasks) != 1 || s.ws.Projects[0].Tasks[0].ID != "global-only" {
		t.Errorf("项目内配置模式不该动注册表里的任务，得到 %+v", s.ws.Projects[0].Tasks)
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
	if !strings.Contains(prompt, "env") {
		t.Error("prompt 里应说明 env 字段，否则模型会把 PORT=xxx 写进 command")
	}
}

// 描述和项目名要出现在 prompt 的输出结构里，否则模型不会给，
// 界面上那行描述永远是空的。
func TestBuildPromptAsksForProjectMeta(t *testing.T) {
	prompt := BuildPrompt(ResolvedProject{Path: `D:\code\my-shop`})
	for _, want := range []string{`"project"`, `"description"`, "【项目信息】"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt 里应包含 %s，否则模型不会输出项目信息", want)
		}
	}
}

// dependsOn / pipelines 目前没有执行方，prompt 里一出现就会让用户配出
// 点了不生效的东西。等 M2 把调度接上再改这个测试。
func TestBuildPromptOmitsUnimplementedFields(t *testing.T) {
	prompt := BuildPrompt(ResolvedProject{Path: `D:\code\my-shop`})
	for _, field := range []string{"dependsOn", "pipelines"} {
		if strings.Contains(prompt, field) {
			t.Errorf("prompt 不该提 %s：这个字段还没有执行方", field)
		}
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
