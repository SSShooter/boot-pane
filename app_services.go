package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"bootpane/internal/config"
	"bootpane/internal/portinfo"
	"bootpane/internal/process"
)

// ConfigService 向前端暴露配置读写能力。
type ConfigService struct {
	store *config.Store
}

// NewConfigService 构造配置服务。
func NewConfigService(s *config.Store) *ConfigService {
	return &ConfigService{store: s}
}

// ListProjects 返回所有项目的合并视图（含校验结果）。
func (c *ConfigService) ListProjects() []config.ResolvedProject {
	return c.store.Projects()
}

// GetProject 返回单个项目。
func (c *ConfigService) GetProject(id string) (config.ResolvedProject, error) {
	return c.store.Get(id)
}

// AddProject 把一个目录登记为项目。
func (c *ConfigService) AddProject(path string) (config.ResolvedProject, error) {
	return c.store.Add(path)
}

// RemoveProject 从注册表移除项目，不影响磁盘上的仓库。
func (c *ConfigService) RemoveProject(id string) error {
	return c.store.Remove(id)
}

// OpenProjectDir 在系统文件管理器里打开项目目录。
func (c *ConfigService) OpenProjectDir(id string) error {
	p, err := c.store.Get(id)
	if err != nil {
		return err
	}
	return revealPath(p.Path)
}

// OpenConfigDir 打开全局配置所在目录。
func (c *ConfigService) OpenConfigDir() error {
	return revealPath(c.store.Dir())
}

// ConfigFilePath 返回注册表文件路径，界面上用于展示。
func (c *ConfigService) ConfigFilePath() string {
	return c.store.FilePath()
}

// SetUseRepoConfig 切换某个项目的配置来源：全局注册表 ↔ 仓库内 .bootpane.json。
func (c *ConfigService) SetUseRepoConfig(id string, enabled bool) error {
	return c.store.SetUseRepoConfig(id, enabled)
}

// BuildConfigPrompt 生成一段可以贴给任意大模型的 prompt，用来产出启动配置。
func (c *ConfigService) BuildConfigPrompt(id string) (string, error) {
	p, err := c.store.Get(id)
	if err != nil {
		return "", err
	}
	return config.BuildPrompt(p), nil
}

// ImportTasks 解析用户粘贴回来的配置，校验通过后才写入全局注册表。
//
// 大模型会编造不存在的命令，所以这一步不能直接落盘：先按项目真实路径跑一遍
// 校验器，有阻断性问题就把问题原样返回，一条都不写。
func (c *ConfigService) ImportTasks(id, raw string) (config.ImportResult, error) {
	p, err := c.store.Get(id)
	if err != nil {
		return config.ImportResult{}, err
	}
	if p.UseRepoConfig {
		return config.ImportResult{Error: "该项目当前使用项目内配置，请先切回全局再导入"}, nil
	}

	pf, err := config.ParseProjectFile(raw)
	if err != nil {
		return config.ImportResult{Error: err.Error()}, nil
	}

	check := config.ResolvedProject{
		Path:      p.Path,
		Exists:    p.Exists,
		Tasks:     pf.Tasks,
		Pipelines: pf.Pipelines,
	}
	issues := config.Validate(&check)
	for _, issue := range issues {
		if issue.Level == "error" {
			return config.ImportResult{
				Error:  "导入的配置有错误，已取消写入",
				Issues: issues,
			}, nil
		}
	}

	if err := c.store.SetTasks(id, pf.Tasks, pf.Pipelines); err != nil {
		return config.ImportResult{}, err
	}
	return config.ImportResult{OK: true, Imported: len(pf.Tasks), Issues: issues}, nil
}

// ProcessService 向前端暴露任务运行控制。
type ProcessService struct {
	mgr   *process.Manager
	store *config.Store
}

// NewProcessService 构造进程服务。
func NewProcessService(m *process.Manager, s *config.Store) *ProcessService {
	return &ProcessService{mgr: m, store: s}
}

// ListRunning 返回所有任务的运行快照。
func (p *ProcessService) ListRunning() []process.Runtime {
	return p.mgr.List()
}

// TaskLog 返回某个任务已缓冲的日志全量。
//
// 前端打开日志面板时先拉一次历史，之后靠 bootpane:log 事件增量追加。
// 用一次性拉取而不是把全量塞进事件，是因为事件通道承载不了几百 KB 的突发。
func (p *ProcessService) TaskLog(projectID, taskID string) string {
	return p.mgr.Log(projectID, taskID)
}

// CheckPorts 批量查询端口占用。
//
// 和 ListRunning 分开是有意的：任务状态只反映「bootpane 启动的那个进程」，
// 端口状态反映「服务是否活着」，两者会不一致。典型场景是启动脚本自己发现
// 「已在运行」就跳过退出——进程退了，但服务是活的，只看任务状态就会误报
// 「已停止」。
func (p *ProcessService) CheckPorts(ports []int) []portinfo.Listener {
	return portinfo.LookupAll(ports)
}

// StartTask 启动项目下的某个任务。
func (p *ProcessService) StartTask(projectID, taskID string) error {
	proj, err := p.store.Get(projectID)
	if err != nil {
		return err
	}
	for _, t := range proj.Tasks {
		if t.ID == taskID {
			return p.mgr.Start(projectID, t, proj.Path)
		}
	}
	return fmt.Errorf("任务 %q 不存在", taskID)
}

// StopTask 停止某个任务及其整棵进程树。
func (p *ProcessService) StopTask(projectID, taskID string) error {
	return p.mgr.Stop(projectID, taskID)
}

// StopProject 停止项目下所有任务。
func (p *ProcessService) StopProject(projectID string) error {
	return p.mgr.StopProject(projectID)
}

// revealPath 调用系统文件管理器定位到指定路径。
func revealPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
