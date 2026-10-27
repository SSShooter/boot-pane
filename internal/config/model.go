// Package config 负责启动器的两层配置：全局注册表与仓库内的 .bootpane.json。
//
// 分两层的原因：哪些项目属于这台机器，不该进仓库；项目怎么启动属于仓库，
// 应该跟代码一起走、供团队共享。
package config

// 配置来源标记，用于在 UI 上区分「团队共享」与「我个人加的」。
const (
	OriginRepo  = "repo"  // 来自仓库内 .bootpane.json
	OriginLocal = "local" // 来自全局注册表的个人覆盖层
)

// WaitFor 的匹配类型。
const (
	WaitPort = "port" // 等待本地端口进入监听状态
	WaitLog  = "log"  // 等待日志中出现指定关键字
)

// Pipeline 的执行模式。
const (
	ModeSerial   = "serial"
	ModeParallel = "parallel"
)

// Workspace 是全局注册表，保存在用户配置目录下，不随仓库分发。
type Workspace struct {
	Version  int       `json:"version"`
	Projects []Project `json:"projects"`
}

// Project 是注册表里的一个项目条目。
//
// LocalTasks 是个人覆盖层：团队配置放在仓库的 .bootpane.json 里，
// 但用户可能想加一个只有自己用的调试按钮，又不希望提交进 git，
// 就写在这里。合并时以仓库配置为准，这里只补缺。
type Project struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Tags       []string `json:"tags,omitempty"`
	Pinned     bool     `json:"pinned,omitempty"`
	LocalTasks []Task   `json:"localTasks,omitempty"`
}

// ProjectFile 是仓库内 .bootpane.json 的结构，建议提交进 git。
type ProjectFile struct {
	Version   int        `json:"version"`
	Project   Meta       `json:"project,omitempty"`
	Tasks     []Task     `json:"tasks"`
	Pipelines []Pipeline `json:"pipelines,omitempty"`
}

// Meta 是仓库自带的项目元信息，注册表里没填时用它兜底。
type Meta struct {
	Name string   `json:"name,omitempty"`
	Tags []string `json:"tags,omitempty"`
}

// Task 是一个启动按钮的全部定义。
type Task struct {
	ID      string            `json:"id"`
	Label   string            `json:"label"`
	Group   string            `json:"group,omitempty"` // 用于按钮分组，如「前端」「小程序」
	Command string            `json:"command"`         // 交给 shell 执行的完整命令
	Cwd     string            `json:"cwd,omitempty"`   // 相对项目根目录
	Env     map[string]string `json:"env,omitempty"`

	// DependsOn 声明前置任务。启动前会等这些任务真正就绪，而不是盲等固定秒数。
	DependsOn []string `json:"dependsOn,omitempty"`

	// WaitFor 定义「什么算启动完成」。没有它就只能靠猜。
	WaitFor *WaitFor `json:"waitFor,omitempty"`

	OpenURL   string `json:"openUrl,omitempty"`   // 就绪后自动打开
	Singleton *bool  `json:"singleton,omitempty"` // 默认 true，防止同一按钮起两份
	AutoStart bool   `json:"autoStart,omitempty"`

	// Origin 由合并阶段填写，不写盘。
	Origin string `json:"origin,omitempty"`
}

// SingletonEnabled 返回是否禁止重复启动，未显式配置时默认为 true。
func (t Task) SingletonEnabled() bool {
	return t.Singleton == nil || *t.Singleton
}

// WaitFor 描述就绪判定条件。
type WaitFor struct {
	Type    string `json:"type"`              // WaitPort | WaitLog
	Port    int    `json:"port,omitempty"`    // Type == port 时使用
	Log     string `json:"log,omitempty"`     // Type == log 时使用
	Timeout int    `json:"timeout,omitempty"` // 秒，0 表示用默认值
}

// Pipeline 把多个任务按顺序或并行组成一键启动。
type Pipeline struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Tasks []string `json:"tasks"`
	Mode  string   `json:"mode,omitempty"`
}

// ResolvedProject 是两层配置合并后的只读视图，直接交给前端渲染。
type ResolvedProject struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Tags       []string `json:"tags"`
	Pinned     bool     `json:"pinned"`
	Exists     bool     `json:"exists"`         // 目录是否还在
	HasRepoCfg bool     `json:"hasRepoConfig"`  // 是否存在 .bootpane.json
	ConfigErr  string   `json:"configError,omitempty"`
	Tasks      []Task     `json:"tasks"`
	Pipelines  []Pipeline `json:"pipelines"`
	Issues     []Issue    `json:"issues"`
}

// Issue 是校验阶段发现的问题。error 级会阻止启动，warn 级只提示。
type Issue struct {
	Level   string `json:"level"` // error | warn
	TaskID  string `json:"taskId,omitempty"`
	Message string `json:"message"`
}

// HasError 判断是否存在阻断性问题。
func (r ResolvedProject) HasError() bool {
	for _, is := range r.Issues {
		if is.Level == "error" {
			return true
		}
	}
	return false
}
