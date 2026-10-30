// Package quitguard 管「关窗口时还有任务在跑」这一档交互。
//
// 它不碰窗口和进程，只拿三个函数：查运行中任务、真正退出、推事件给前端。
// 这样状态机能单独测——不用起应用，也不用真去关一个窗口。
package quitguard

import (
	"sync"
	"time"

	"bootpane/internal/process"
)

// EventCloseRequest 在用户关窗口、但还有任务在运行时推给前端。
//
// 这一次关闭已经被拦下来了，前端要么弹确认框（先回执 AcknowledgeClose），
// 要么什么都不做——超时后按「停止并退出」兜底，免得界面挂了连窗口都关不掉。
const EventCloseRequest = "bootpane:close-request"

// Request 是 EventCloseRequest 的负载。
//
// 只带 id 不带标题：前端本来就有完整的项目和任务列表，翻译成中文名
// 是它的事，后端不用为了拼一句话去反查配置。
type Request struct {
	Tasks []Task `json:"tasks"`
}

// Task 是一个正在运行的任务的定位信息。
type Task struct {
	ProjectID string `json:"projectId"`
	TaskID    string `json:"taskId"`
}

// closeAckTimeout 是等前端回执的上限。
//
// 正常路径下回执是「收到事件立刻发」，几百毫秒内就到；超过这个时间说明
// webview 已经没了或还没加载完，再拦着关闭只会让用户没法退出应用。
const closeAckTimeout = 5 * time.Second

// Cancelable 是窗口关闭事件里这里用到的最小接口。
//
// 只取 Cancel：guard 不关心事件来自哪种窗口，测试也就不用伪造 Wails 的
// 事件对象。Wails 的 *application.WindowEvent 天然满足它。
type Cancelable interface {
	Cancel()
}

// Guard 在窗口关闭前拦一道：有任务在跑就先问用户。
//
// 不用系统对话框：界面里所有确认都走 ConfirmDialog，主题字体一致；
// 系统弹窗画在窗口之外，风格和按钮都跟界面对不上。
type Guard struct {
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

// New 构造一个关闭守卫。
func New(running func() []process.Runtime, quit func(), emit func(string, any)) *Guard {
	return &Guard{
		running:    running,
		quit:       quit,
		emit:       emit,
		ackTimeout: closeAckTimeout,
	}
}

// HandleClosing 是 WindowClosing 钩子的主体。
//
// 钩子跑在 Wails 的窗口事件 goroutine 里（窗口事件先进 channel 再分发），
// 所以这里只负责拦下关闭，等回执和弹框都丢给 prompt 那个 goroutine，
// 不占住事件分发。
func (g *Guard) HandleClosing(e Cancelable) {
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
func (g *Guard) prompt(running []process.Runtime, ack chan struct{}) {
	tasks := make([]Task, 0, len(running))
	for _, r := range running {
		tasks = append(tasks, Task{ProjectID: r.ProjectID, TaskID: r.TaskID})
	}
	g.emit(EventCloseRequest, Request{Tasks: tasks})

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
func (g *Guard) AcknowledgeClose() {
	g.mu.Lock()
	ack := g.ack
	g.ack = nil
	g.mu.Unlock()

	if ack != nil {
		close(ack)
	}
}

// ConfirmQuit 用户确认「停止并退出」。
func (g *Guard) ConfirmQuit() {
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
func (g *Guard) CancelQuit() {
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
