package main

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"time"

	"bootpane/internal/config"
	"bootpane/internal/portinfo"
	"bootpane/internal/process"
	"bootpane/internal/prockill"
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

// SetStar 设置任务星标。
//
// 星标只落在本机全局注册表里，不写进仓库配置——它是个人使用习惯，
// 不该跟着 .bootpane.json 提交给团队。
func (c *ConfigService) SetStar(projectID, taskID string, starred bool) error {
	return c.store.SetStar(projectID, taskID, starred)
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

// RestartResult 是一次重启操作的结果。
//
// 和 ImportResult 一样用返回值而不是 error 承载业务结果：Wails 的 (T, error)
// 约定下 error 非空时 T 会被丢弃，前端就拿不到「杀了哪些进程」这些细节。
type RestartResult struct {
	OK      bool     `json:"ok"`
	Killed  []string `json:"killed"` // 形如 "node.exe (PID 47772)"
	Message string   `json:"message"`
}

// 等待端口释放的上限。
//
// TerminateProcess 是异步的：调用返回时进程可能还在退出流程里，端口没断。
// 直接启动会撞上 EADDRINUSE，所以必须等端口真的空出来。
const portReleaseTimeout = 8 * time.Second

// RestartTask 重启任务：端口被外部进程占着时，先杀掉那棵进程树再启动。
//
// 这里的「外部进程」完全不在我们的 Job Object 里，只能靠端口反查 PID
// 再用 Toolhelp 摸父子关系。杀之前前端已经做过二次确认。
func (p *ProcessService) RestartTask(projectID, taskID string) RestartResult {
	proj, err := p.store.Get(projectID)
	if err != nil {
		return RestartResult{Message: err.Error()}
	}

	var task *config.Task
	for i := range proj.Tasks {
		if proj.Tasks[i].ID == taskID {
			task = &proj.Tasks[i]
			break
		}
	}
	if task == nil {
		return RestartResult{Message: fmt.Sprintf("任务 %q 不存在", taskID)}
	}

	// 只有配了 waitFor.port 的任务才可能被判定成「外部运行中」，
	// 没有端口就没有「占着端口的外部实例」这个概念。
	port := 0
	if task.WaitFor != nil && task.WaitFor.Type == config.WaitPort {
		port = task.WaitFor.Port
	}
	if port <= 0 {
		return RestartResult{Message: "该任务没有配置端口，无法判断外部实例"}
	}

	var killed []string
	if listener := portinfo.Lookup(port); listener.Listening && listener.PID > 0 {
		victims, killErr := prockill.Kill(listener.PID)
		if killErr != nil {
			return RestartResult{
				Message: fmt.Sprintf("终止 %s 失败：%v", describeListener(listener), killErr),
			}
		}
		for _, v := range victims {
			killed = append(killed, describeKilled(v))
		}
		if err := waitPortRelease(port, portReleaseTimeout); err != nil {
			return RestartResult{
				Killed:  killed,
				Message: fmt.Sprintf("已终止 %d 个进程，但端口 %d 仍被占用", len(killed), port),
			}
		}
	}

	if err := p.StartTask(projectID, taskID); err != nil {
		return RestartResult{Killed: killed, Message: err.Error()}
	}

	if len(killed) == 0 {
		return RestartResult{OK: true, Message: "已启动"}
	}
	return RestartResult{OK: true, Killed: killed, Message: "已终止外部实例并重新启动"}
}

// waitPortRelease 轮询到端口不再能建立连接。
func waitPortRelease(port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return nil // 连不上就说明已经释放了
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("端口 %d 在 %v 内没有释放", port, timeout)
}

func describeListener(l portinfo.Listener) string {
	if l.Process != "" {
		return fmt.Sprintf("%s（PID %d）", l.Process, l.PID)
	}
	return fmt.Sprintf("PID %d", l.PID)
}

func describeKilled(k prockill.Killed) string {
	if k.Name != "" {
		return fmt.Sprintf("%s (PID %d)", k.Name, k.PID)
	}
	return fmt.Sprintf("PID %d", k.PID)
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
