// Package config 负责启动器的两层配置：全局注册表与仓库内的 .bootpane.json。
//
// 分两层的原因：哪些项目属于这台机器，不该进仓库；项目怎么启动属于仓库，
// 应该跟代码一起走、供团队共享。
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// 配置来源。两者二选一，不叠加。
const (
	SourceGlobal = "global" // 来自全局注册表，默认行为
	SourceRepo   = "repo"   // 来自仓库内 .bootpane.json
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
	Stars    []StarRef `json:"stars,omitempty"`
	Settings Settings  `json:"settings"`
}

// Settings 是本机偏好，存在全局注册表的顶层。
//
// 和 StarRef 一样，这些是「这台机器上的使用习惯」，不能写进随仓库分发的
// .bootpane.json：切到项目内配置时那些字段无处落地，还会被提交给团队。
type Settings struct {
	// AutoOpenLogOnStart 启动任务后自动展开该任务的日志抽屉。
	// 零值 false 表示不自动打开，与老版本行为一致；旧配置文件里没有
	// settings 字段时反序列化后就是这个零值，不需要额外迁移。
	AutoOpenLogOnStart bool `json:"autoOpenLogOnStart"`

	// OpenWith 是「打开目录」按钮执行的命令，空字符串表示交给系统文件管理器。
	//
	// 存完整命令行而不是 IDE 的枚举值：用户可能用 VS Code、Cursor，
	// 也可能是自己写的脚本，枚举永远列不全。命令里可以写 {path} 占位符，
	// 不写时项目路径自动追加到末尾——多数编辑器 CLI 正是这个用法。
	OpenWith string `json:"openWith,omitempty"`
}

// StarRef 指向一个被星标的任务。
//
// 星标是「本机视角的偏好」，不是任务定义的一部分，所以单独存一份，
// 不往 Task 里塞字段。原因是项目内配置（.bootpane.json）要跟仓库走、
// 由团队共享，星标这种纯个人习惯的东西一旦混进任务定义，切到项目内配置
// 时就无处落地了。独立列表对两种配置来源一视同仁。
type StarRef struct {
	ProjectID string `json:"projectId"`
	TaskID    string `json:"taskId"`
}

// Project 是注册表里的一个项目条目。
//
// 默认情况下启动按钮全部存在这里，用户不需要往仓库里塞任何文件。
// 只有显式把 UseRepoConfig 打开，才会去读仓库根目录的 .bootpane.json。
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Description 是一句话说明，显示在项目头部。可以为空。
	Description   string     `json:"description,omitempty"`
	Path          string     `json:"path"`
	Tags          []string   `json:"tags,omitempty"`
	Pinned        bool       `json:"pinned,omitempty"`
	UseRepoConfig bool       `json:"useRepoConfig,omitempty"`
	Tasks         []Task     `json:"tasks,omitempty"`
	Pipelines     []Pipeline `json:"pipelines,omitempty"`
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
	Name string `json:"name,omitempty"`
	// Description 是这个项目干什么的一句话说明，供界面显示。
	// 和 Name 一样，注册表里有值时以注册表为准。
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
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

	OpenURL   string `json:"openUrl,omitempty"`   // 页面地址，配了就在卡片上显示「打开页面」
	Singleton *bool  `json:"singleton,omitempty"` // 默认 true，防止同一按钮起两份
	AutoStart bool   `json:"autoStart,omitempty"`
}

// SingletonEnabled 返回是否禁止重复启动，未显式配置时默认为 true。
func (t Task) SingletonEnabled() bool {
	return t.Singleton == nil || *t.Singleton
}

// frontendHints 是判断「这条任务能直接用浏览器看」的关键词。
//
// 只扫 id / label / group，不扫 command：`go run ./cmd/web` 这种后端命令里
// 同样有 web，按命令匹配会把 API 也挂上「打开页面」。关键词只是兜底，
// 准确的做法是让模型在生成配置时显式写 openUrl（prompt 里已经要求了）。
var frontendHints = []string{
	"前端", "网页", "web", "frontend", "front-end", "h5",
	"vite", "vue", "react", "svelte", "nuxt", "next", "angular", "astro",
}

// backendHints 优先于 frontendHints 判断。
//
// 「启动 Web API」这类任务同时命中了 web，但它返回的是 JSON，不该挂页面按钮。
// 宁可漏掉一个，也不要在 API 上放一个点开只有一坨花括号的按钮。
var backendHints = []string{"后端", "服务端", "api", "mock"}

// IsFrontend 判断任务是不是一个能对着浏览器看的页面入口。
func (t Task) IsFrontend() bool {
	haystack := strings.ToLower(t.ID + " " + t.Label + " " + t.Group)
	for _, hint := range backendHints {
		if strings.Contains(haystack, hint) {
			return false
		}
	}
	for _, hint := range frontendHints {
		if strings.Contains(haystack, hint) {
			return true
		}
	}
	return false
}

// PageURL 返回任务对应的网页地址，解析不出时返回空串。
//
// 显式配置的 openUrl 优先：反代后的域名、https、带路径的地址都没法从端口
// 推导出来。没配时才在前端类任务 + 监听端口的情况下用 http://localhost:<port>
// 兜底——后端 API 也监听端口，给它们挂一个打开按钮只会误导用户。
func (t Task) PageURL() string {
	if u := strings.TrimSpace(t.OpenURL); u != "" {
		if IsHTTPURL(u) {
			return u
		}
		// 显式配了但地址不合法：不挂按钮，Validate 里会给出提示。
		// 这里不能退回端口推导，否则用户改了地址却看不到任何变化。
		return ""
	}
	if !t.IsFrontend() || t.WaitFor == nil || t.WaitFor.Type != WaitPort || t.WaitFor.Port <= 0 {
		return ""
	}
	return fmt.Sprintf("http://localhost:%d", t.WaitFor.Port)
}

// IsHTTPURL 判断是不是可以直接交给浏览器打开的 http(s) 地址。
//
// 配置来自 JSON，不能拿 file:// 或自定义协议去塞给系统，否则等于给配置
// 文件开了一个任意程序启动的口子。
func IsHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
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

// ResolvedProject 是配置解析后的只读视图，直接交给前端渲染。
type ResolvedProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Description 可能来自注册表，也可能来自仓库 meta，可能为空。
	Description   string     `json:"description"`
	Path          string     `json:"path"`
	Tags          []string   `json:"tags"`
	Pinned        bool       `json:"pinned"`
	Exists        bool       `json:"exists"` // 目录是否还在
	UseRepoConfig bool       `json:"useRepoConfig"`
	ConfigSource  string     `json:"configSource"`  // SourceGlobal | SourceRepo
	HasRepoCfg    bool       `json:"hasRepoConfig"` // 仓库里是否真的存在 .bootpane.json
	ConfigErr     string     `json:"configError,omitempty"`
	Tasks         []Task     `json:"tasks"`
	Pipelines     []Pipeline `json:"pipelines"`
	Issues        []Issue    `json:"issues"`
	// StarredTaskIDs 是本项目下被星标的任务 id。
	// 前端用它判断哪张卡片是星标，跨项目聚合时也靠它筛选。
	StarredTaskIDs []string `json:"starredTaskIds"`
	// PageURLs 是「任务 id → 网页地址」，只包含能解析出地址的任务。
	// 前端据此决定要不要显示「打开页面」按钮，规则和 OpenTaskPage 共用一份，
	// 不会出现按钮显示着却打不开的情况。
	PageURLs map[string]string `json:"pageUrls,omitempty"`
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
