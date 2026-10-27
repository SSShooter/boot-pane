package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bootpane/internal/config"
)

// newTestConfigService 构造一个不碰真实用户配置目录的 ConfigService。
func newTestConfigService(t *testing.T) (*ConfigService, *config.Store) {
	t.Helper()
	store, err := config.NewStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}
	return NewConfigService(store), store
}

// 编辑框保存成功后，任务要真的落到注册表里。
func TestSaveConfigJSONUpdatesTasks(t *testing.T) {
	svc, store := newTestConfigService(t)
	proj, err := store.Add(t.TempDir())
	if err != nil {
		t.Fatalf("添加项目失败: %v", err)
	}

	raw := `{
		"version": 1,
		"project": {"name": "改过的名字", "tags": ["go"]},
		"tasks": [
			{"id": "api", "label": "启动 API", "command": "go run ./cmd/api"}
		]
	}`
	res, err := svc.SaveConfigJSON(proj.ID, raw)
	if err != nil {
		t.Fatalf("保存不该返回系统错误: %v", err)
	}
	if !res.OK {
		t.Fatalf("保存失败: %s %+v", res.Error, res.Issues)
	}

	got, err := store.Get(proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].ID != "api" {
		t.Fatalf("任务没有写进注册表，得到 %+v", got.Tasks)
	}
	// 编辑框里显示了项目名，保存后就该是这个值
	if got.Name != "改过的名字" {
		t.Errorf("项目名应跟着编辑框走，得到 %q", got.Name)
	}
}

// 校验不过的配置一条都不能写——包括「删掉的任务还被 pipeline 引用」这种
// 看着像删除、其实是把配置改坏的操作。
func TestSaveConfigJSONRejectsInvalid(t *testing.T) {
	svc, store := newTestConfigService(t)
	proj, err := store.Add(t.TempDir())
	if err != nil {
		t.Fatalf("添加项目失败: %v", err)
	}
	original := []config.Task{{ID: "web", Label: "前端", Command: "pnpm dev"}}
	if err := store.SetTasks(proj.ID, original, nil); err != nil {
		t.Fatalf("写入初始任务失败: %v", err)
	}

	// 命令为空 → error
	res, err := svc.SaveConfigJSON(proj.ID, `{"version":1,"tasks":[{"id":"bad","label":"空的"}]}`)
	if err != nil {
		t.Fatalf("校验失败不该走 error 返回: %v", err)
	}
	if res.OK {
		t.Fatal("空命令的配置不该保存成功")
	}
	if res.Error == "" {
		t.Error("应给出失败原因")
	}

	got, err := store.Get(proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].ID != "web" {
		t.Errorf("校验失败时不该改动已有配置，得到 %+v", got.Tasks)
	}
}

// 删光所有启动项是合法操作，不能被「配置里没有任何 task」挡住。
func TestSaveConfigJSONAllowsEmptyTasks(t *testing.T) {
	svc, store := newTestConfigService(t)
	proj, err := store.Add(t.TempDir())
	if err != nil {
		t.Fatalf("添加项目失败: %v", err)
	}
	if err := store.SetTasks(proj.ID, []config.Task{{ID: "web", Command: "pnpm dev"}}, nil); err != nil {
		t.Fatalf("写入初始任务失败: %v", err)
	}

	res, err := svc.SaveConfigJSON(proj.ID, `{"version":1,"tasks":[]}`)
	if err != nil {
		t.Fatalf("保存空配置不该返回系统错误: %v", err)
	}
	if !res.OK {
		t.Fatalf("删光启动项应当允许保存: %s", res.Error)
	}

	got, err := store.Get(proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tasks) != 0 {
		t.Errorf("任务应被清空，得到 %+v", got.Tasks)
	}
}

// 项目内配置模式下，保存写的是仓库文件而不是注册表。
func TestSaveConfigJSONWritesRepoFile(t *testing.T) {
	svc, store := newTestConfigService(t)
	repo := t.TempDir()
	proj, err := store.Add(repo)
	if err != nil {
		t.Fatalf("添加项目失败: %v", err)
	}
	if err := store.SetUseRepoConfig(proj.ID, true); err != nil {
		t.Fatalf("切换配置来源失败: %v", err)
	}

	raw := `{"version":1,"project":{"name":"仓库项目"},"tasks":[{"id":"only","label":"唯一","command":"echo hi"}]}`
	res, err := svc.SaveConfigJSON(proj.ID, raw)
	if err != nil {
		t.Fatalf("保存不该返回系统错误: %v", err)
	}
	if !res.OK {
		t.Fatalf("保存失败: %s %+v", res.Error, res.Issues)
	}

	data, err := os.ReadFile(filepath.Join(repo, config.RepoConfigName))
	if err != nil {
		t.Fatalf("应写入仓库配置文件: %v", err)
	}
	var pf config.ProjectFile
	if err := json.Unmarshal(data, &pf); err != nil {
		t.Fatalf("写出的文件应是合法 JSON: %v", err)
	}
	if len(pf.Tasks) != 1 || pf.Tasks[0].ID != "only" {
		t.Errorf("仓库文件内容不对，得到 %+v", pf.Tasks)
	}
}

// ConfigJSON 要返回当前真正生效的那一份，界面才能「打开就是对的」。
func TestConfigJSONMatchesActiveSource(t *testing.T) {
	svc, store := newTestConfigService(t)
	repo := t.TempDir()
	proj, err := store.Add(repo)
	if err != nil {
		t.Fatalf("添加项目失败: %v", err)
	}
	if err := store.SetTasks(proj.ID, []config.Task{{ID: "global", Command: "echo g"}}, nil); err != nil {
		t.Fatalf("写入任务失败: %v", err)
	}

	raw, err := svc.ConfigJSON(proj.ID)
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	var pf config.ProjectFile
	if err := json.Unmarshal([]byte(raw), &pf); err != nil {
		t.Fatalf("应返回合法 JSON: %v\n%s", err, raw)
	}
	if len(pf.Tasks) != 1 || pf.Tasks[0].ID != "global" {
		t.Errorf("全局模式下应返回注册表里的任务，得到 %+v", pf.Tasks)
	}

	// 切到项目内配置，仓库里还没有文件 → 应给出空骨架而不是报错
	if err := store.SetUseRepoConfig(proj.ID, true); err != nil {
		t.Fatalf("切换配置来源失败: %v", err)
	}
	raw, err = svc.ConfigJSON(proj.ID)
	if err != nil {
		t.Fatalf("仓库没有配置文件时应返回空骨架，而不是报错: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &pf); err != nil {
		t.Fatalf("骨架应是合法 JSON: %v\n%s", err, raw)
	}
	if len(pf.Tasks) != 0 {
		t.Errorf("仓库里没有任务，骨架不该凭空造，得到 %+v", pf.Tasks)
	}
}
