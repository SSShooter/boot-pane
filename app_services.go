package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"bootpane/internal/config"
	"bootpane/internal/editors"
	"bootpane/internal/host"
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

// OpenProjectDir 用系统文件管理器或设置里指定的编辑器打开项目目录。
func (c *ConfigService) OpenProjectDir(id string) error {
	p, err := c.store.Get(id)
	if err != nil {
		return err
	}
	if command := c.store.Settings().OpenWith; command != "" {
		return host.LaunchOpenWith(command, p.Path)
	}
	return host.Reveal(p.Path)
}

// SetOpenWith 设置「打开目录」使用的命令，传空字符串表示回到系统文件管理器。
//
// 保存前先确认命令能找到：拼错的命令等到点按钮才报错，用户很难把错误
// 和设置页里那一行输入联系起来。
func (c *ConfigService) SetOpenWith(command string) error {
	command = strings.TrimSpace(command)
	if command != "" {
		if _, err := host.LookupOpenWith(command); err != nil {
			return err
		}
	}
	return c.store.SetOpenWith(command)
}

// DetectEditors 扫描本机装了哪些编辑器，供设置页直接选。
//
// 每次调用都重新扫，不做缓存：用户刚装完编辑器就来点「重新扫描」，
// 返回旧列表比返回空列表更让人困惑。探测本身只是若干次 PATH 查找和 stat。
func (c *ConfigService) DetectEditors() []editors.Editor {
	return editors.Detect()
}

// OpenConfigDir 打开全局配置所在目录。
func (c *ConfigService) OpenConfigDir() error {
	return host.Reveal(c.store.Dir())
}

// OpenTaskPage 在系统默认浏览器里打开任务对应的页面。
//
// 地址解析复用 config.Task.PageURL，和前端决定「要不要显示按钮」是同一套
// 规则，不会出现按钮点下去却打不开的情况。前端没传地址而是传任务 id，
// 也是为了让配置改了之后行为跟着变，不用两边各存一份 URL。
func (c *ConfigService) OpenTaskPage(id, taskID string) error {
	p, err := c.store.Get(id)
	if err != nil {
		return err
	}
	for _, t := range p.Tasks {
		if t.ID != taskID {
			continue
		}
		page := t.PageURL()
		if page == "" {
			return fmt.Errorf("任务 %q 没有可打开的页面", t.Label)
		}
		return host.OpenExternalURL(page)
	}
	return fmt.Errorf("任务 %q 不存在", taskID)
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

// GetSettings 返回本机偏好（自动打开日志这类开关）。
//
// 偏好落在 workspace.json 顶层，和星标一样属于「这台机器的习惯」，
// 不跟着 .bootpane.json 走。
func (c *ConfigService) GetSettings() config.Settings {
	return c.store.Settings()
}

// SetAutoOpenLogOnStart 切换「启动任务后自动打开日志」。
func (c *ConfigService) SetAutoOpenLogOnStart(enabled bool) error {
	return c.store.SetAutoOpenLogOnStart(enabled)
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

	// 导入会把 JSON 里的项目名和描述写进注册表：prompt 已经要求模型输出这两项，
	// 模型给了却不用，界面上看到的还是旧名字、旧描述。空值视为模型没给，保留原值；
	// 标签不在输出约定里，不动。
	return c.applyConfig(p, pf, false, "导入的配置有错误，已取消写入")
}

// ConfigJSON 返回当前生效配置的完整 JSON 文本，供界面直接编辑。
//
// 返回的是「当前真正生效的那一份」：项目内配置模式读仓库文件原文，
// 全局模式从注册表拼出来。界面不需要知道区别。
func (c *ConfigService) ConfigJSON(id string) (string, error) {
	return c.store.ConfigSnapshot(id)
}

// SaveConfigJSON 保存界面上编辑过的配置。
//
// 和 ImportTasks 共用同一条校验链路，但有两处不同：允许任务列表为空
// （用户在编辑框里把启动项全删了是正常操作），以及项目名/描述/标签跟着编辑框走
// （编辑框里显示了什么，保存后就该是什么，包括把描述删空）。
func (c *ConfigService) SaveConfigJSON(id, raw string) (config.ImportResult, error) {
	p, err := c.store.Get(id)
	if err != nil {
		return config.ImportResult{}, err
	}

	pf, err := config.ParseEditableConfig(raw)
	if err != nil {
		return config.ImportResult{Error: err.Error()}, nil
	}

	return c.applyConfig(p, pf, true, "配置有错误，未保存")
}

// applyConfig 是「导入」和「编辑保存」共用的校验 + 落盘逻辑。
//
// 两条路径都走这里，是为了不出现「导入会被校验拦住、编辑却能写进去」这种
// 两套行为。syncMeta 决定 project 里的元信息怎么写回注册表：true 时名称、
// 描述、标签全以传入的配置为准（编辑框里有什么就是什么）；false 时只取
// 名称和描述的非空值，标签不动。
func (c *ConfigService) applyConfig(p config.ResolvedProject, pf config.ProjectFile, syncMeta bool, failMsg string) (config.ImportResult, error) {
	// 校验要按项目的真实路径跑：工作目录存不存在、依赖指向的任务在不在，
	// 这些都只有拿到磁盘上的实际情况才判得准。
	check := config.ResolvedProject{
		Path:      p.Path,
		Exists:    p.Exists,
		Tasks:     pf.Tasks,
		Pipelines: pf.Pipelines,
	}
	issues := config.Validate(&check)
	for _, issue := range issues {
		if issue.Level == "error" {
			return config.ImportResult{Error: failMsg, Issues: issues}, nil
		}
	}

	if err := c.store.ApplyConfig(p.ID, pf, syncMeta); err != nil {
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
