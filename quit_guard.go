package main

import (
	"sync"
	"time"

	"bootpane/internal/process"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventCloseRequest 在用户关窗口、但还有任务在运行时推给前端。
//
// 这一次关闭已经被拦下来了，前端要么弹确认框（先回执 AcknowledgeClose），
// 要么什么都不做——超时后按「停止并退出」兜底，免得界面挂了连窗口都关不掉。
const EventCloseRequest = "bootpane:close-request"

// CloseRequest 是 EventCloseRequest 的负载。
//
// 只带 id 不带标题：前端本来就有完整的项目和任务列表，翻译成中文名
// 是它的事，后端不用为了拼一句话去反查配置。
type CloseRequest struct {
	Tasks []CloseTask `json:"tasks"`
}

// CloseTask 是一个正在运行的任务的定位信息。
type CloseTask struct {
	ProjectID string `json:"projectId"`
	TaskID    string `json:"taskId"`
}

// closeAckTimeout 是等前端回执的上限。
//
// 正常路径下回执是「收到事件立刻发」，几百毫秒内就到；超过这个时间说明
// webview 已经没了或还没加载完，再拦着关闭只会让用户没法退出应用。
const closeAckTimeout = 5 * time.Second

// quitGuard 在窗口关闭前拦一道：有任务在跑就先问用户。
//
// 不用系统对话框：界面里所有确认都走 ConfirmDialog，主题字体一致；
// 系统弹窗画在窗口之外，风格和按钮都跟界面对不上。
type quitGuard struct {
	// running 返回当前在跑的任务快照。抽成函数是为了单测能塞假数据，
	// 不用真的去起一个进程。
	running func() []process.Runtime
	// quit 是真正退出应用的入口（main 里接到 app.Quit 上）。
	quit func()
	// emit 把事件推给前端。
	emit func(event string, data any)

	// ackTimeout 可覆盖，单测里调小，免得每个用例等 5 秒。
	ackTimeout time.Duration

	mu        sync.Mutex
	prompting bool // 确认框已经弹出来了，等着用户点
	confirmed bool // 用户已经确认退出
	ack       chan struct{}
}

func newQuitGuard(running func() []process.Runtime, quit func(), emit func(string, any)) *quitGuard {
	return &quitGuard{
		running:    running,
		quit:       quit,
		emit:       emit,
		ackTimeout: closeAckTimeout,
	}
}

// HandleWindowClosing 是 WindowClosing 钩子。
//
// 钩子跑在 Wails 的窗口事件 goroutine 里（窗口事件先进 channel 再分发），
// 所以这里只负责拦下关闭，等回执和弹框都丢给 prompt 那个 goroutine，
// 不占住事件分发。
func (g *quitGuard) HandleWindowClosing(e *application.WindowEvent) {
	g.mu.Lock()
	switch {
	case g.confirmed:
		// 已经确认退出，这次放行，让窗口真的关掉。
		g.mu.Unlock()
		return
	case g.prompting:
		// 确认框已经开着，重复点关闭就继续拦着，别叠第二个框。
		g.mu.Unlock()
		e.Cancel()
		return
	}

	running := g.running()
	if len(running) == 0 {
		g.mu.Unlock()
		return
	}

	ack := make(chan struct{})
	g.prompting = true
	g.ack = ack
	g.mu.Unlock()

	// 先拦下这次关闭，等用户在前端做完选择。
	e.Cancel()

	go g.prompt(running, ack)
}

// prompt 把确认请求推给前端，并等它回执。
func (g *quitGuard) prompt(running []process.Runtime, ack chan struct{}) {
	tasks := make([]CloseTask, 0, len(running))
	for _, r := range running {
		tasks = append(tasks, CloseTask{ProjectID: r.ProjectID, TaskID: r.TaskID})
	}
	g.emit(EventCloseRequest, CloseRequest{Tasks: tasks})

	select {
	case <-ack:
		// 界面已经接管，接下来等用户点按钮，等多久都行。
	case <-time.After(g.ackTimeout):
		// 界面没回执：webview 挂了或者还没加载完。这时再拦着关闭，
		// 用户就只能去任务管理器杀进程了，直接按「停止并退出」处理。
		g.ConfirmQuit()
	}
}

// AcknowledgeClose 由前端在收到 EventCloseRequest 后立刻调用，表示确认框已经弹出来了。
func (g *quitGuard) AcknowledgeClose() {
	g.mu.Lock()
	ack := g.ack
	g.ack = nil
	g.mu.Unlock()

	if ack != nil {
		close(ack)
	}
}

// ConfirmQuit 用户确认「停止并退出」。
func (g *quitGuard) ConfirmQuit() {
	g.mu.Lock()
	if g.confirmed {
		g.mu.Unlock()
		return
	}
	g.confirmed = true
	g.prompting = false
	ack := g.ack
	g.ack = nil
	g.mu.Unlock()

	if ack != nil {
		close(ack)
	}

	// 走 app.Quit 而不是直接关窗口：它会跑 OnShutdown，
	// 把每个任务的整棵进程树一起清掉，不留占着端口的孤儿 node。
	g.quit()
}

// CancelQuit 用户选择继续使用：窗口留着，下次再点关闭会重新提示。
func (g *quitGuard) CancelQuit() {
	g.mu.Lock()
	g.prompting = false
	ack := g.ack
	g.ack = nil
	g.mu.Unlock()

	// 关掉回执通道，让 prompt 里的超时兜底停下来——用户已经答过了，
	// 只是答案是「不关」。
	if ack != nil {
		close(ack)
	}
}

// AppService 把「关闭应用」这一侧的交互暴露给前端。
//
// 单独一个服务而不是塞进 ProcessService：它管的是应用生命周期，
// 和启动/停止任务不是一回事。
type AppService struct {
	guard *quitGuard
}

// NewAppService 构造应用生命周期服务。
func NewAppService(g *quitGuard) *AppService {
	return &AppService{guard: g}
}

// AcknowledgeClose 表示界面已经收到关闭提示并弹出了确认框。
func (s *AppService) AcknowledgeClose() {
	s.guard.AcknowledgeClose()
}

// ConfirmQuit 用户确认停止所有任务并退出。
func (s *AppService) ConfirmQuit() {
	s.guard.ConfirmQuit()
}

// CancelQuit 用户取消了这次关闭。
func (s *AppService) CancelQuit() {
	s.guard.CancelQuit()
}
