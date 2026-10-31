package main

import (
	"errors"
	"net/url"
	"strings"
	"sync"

	"bootpane/internal/i18n"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// EventLogWindowClosed 在日志独立窗口关掉后推给前端。
//
// 不管是点了「内嵌」还是直接叉掉窗口，结果都一样：面板只能存在于一个地方，
// 主窗口收到它就把内嵌面板接回来。前端不区分这两种关闭方式。
const EventLogWindowClosed = "bootpane:log-window-closed"

// logWindowName 是日志独立窗口的固定名字。
//
// 同一时刻只允许一个：面板是一个整体（所有标签都在里面），多开之后
// 「新日志该进哪个窗口」就没有答案了。
const logWindowName = "log-window"

// logWindowQuery 是独立窗口入口地址里承载面板状态的参数名。
//
// 必须和前端 lib/events.ts 里的 LOG_WINDOW_QUERY 逐字一致。
const logWindowQuery = "log"

// logWindowURL 把面板状态编码成独立窗口的入口地址。
//
// 状态里只有标签和当前标签（键、项目 id、任务 id），不带日志正文：
// 独立窗口订阅同一路 bootpane:log 事件，历史从后端环形缓冲补一次，
// URL 不该承载几百 KB 的输出。
//
// 走查询参数而不是单独路由，是因为打包后前端资源是 embed 进来的静态文件，
// 多一条路由就要在资源目录里伪造一份对应结构。
func logWindowURL(state string) string {
	return "/?" + logWindowQuery + "=" + url.QueryEscape(state)
}

// LogWindowService 负责把整块日志面板弹成独立窗口。
type LogWindowService struct {
	mu  sync.Mutex
	app *application.App
}

// NewLogWindowService 构造日志窗口服务。
func NewLogWindowService() *LogWindowService {
	return &LogWindowService{}
}

// attach 在 application.New 之后绑定应用实例。
//
// 服务要在 Options 里注册（New 之前），而窗口 API 要 New 之后才存在，
// 这段时序只能靠一个显式的 attach 补上，不能去读全局单例。
//
// 故意不导出：导出就会被绑到前端，而 *application.App 只是 Go 侧的依赖，
// 让生成器把整个 Wails 应用对象连同它的依赖一起翻译成 TS 没有任何意义。
func (s *LogWindowService) attach(app *application.App) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.app = app
}

// appRef 取已经绑定的应用实例。
func (s *LogWindowService) appRef() *application.App {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.app
}

// Open 打开日志独立窗口；已经开着就拉到前面。
//
// state 是前端序列化的面板快照，直接进 URL：新窗口第一次渲染就带着完整的
// 标签和当前标签，不需要等主窗口再推一次状态（那样会先闪一下空面板）。
func (s *LogWindowService) Open(state string) error {
	state = strings.TrimSpace(state)
	if state == "" {
		return errors.New(i18n.T("logWindow.stateEmpty"))
	}

	app := s.appRef()
	if app == nil {
		return errors.New(i18n.T("logWindow.appNotReady"))
	}

	if win, ok := app.Window.GetByName(logWindowName); ok {
		win.Show()
		win.Focus()
		return nil
	}

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      logWindowName,
		Title:     i18n.T("logWindow.title"),
		Width:     880,
		Height:    520,
		MinWidth:  480,
		MinHeight: 220,
		// 和主窗口同色，也和日志正文的 --surface-sunk 接近：这个窗口一开就是日志。
		BackgroundColour: application.NewRGB(10, 10, 12),
		URL:              logWindowURL(state),
	})

	// 钩子跑在关窗流程里，通知主窗口交给别的 goroutine：这里等事件分发
	// 会把关窗拖住，运气不好还会和窗口列表锁纠缠在一起。
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		go app.Event.Emit(EventLogWindowClosed)
	})

	return nil
}

// Focus 把已经开着的日志窗口拉到前面，返回窗口是否还在。
//
// 主窗口的「日志」入口在弹出状态下走这里：用户点它就是想看日志，
// 不是想在主窗口里再开一份。返回 false 说明窗口已经被关掉，
// 前端据此把面板收回内嵌。
func (s *LogWindowService) Focus() bool {
	app := s.appRef()
	if app == nil {
		return false
	}

	win, ok := app.Window.GetByName(logWindowName)
	if !ok {
		return false
	}
	win.Show()
	win.Focus()
	return true
}
