package main

import (
	"sync"
	"testing"
	"time"

	"bootpane/internal/process"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fakeEnv 记录 guard 对外产生的副作用，替代真实的进程管理器和 app.Quit。
type fakeEnv struct {
	mu       sync.Mutex
	runs     []process.Runtime
	quits    int
	requests []CloseRequest
	events   []string
}

func (e *fakeEnv) setRunning(runs ...process.Runtime) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runs = runs
}

func (e *fakeEnv) running() []process.Runtime {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]process.Runtime(nil), e.runs...)
}

func (e *fakeEnv) quit() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.quits++
}

func (e *fakeEnv) emit(event string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
	if req, ok := data.(CloseRequest); ok {
		e.requests = append(e.requests, req)
	}
}

func (e *fakeEnv) counts() (quits, events int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.quits, len(e.events)
}

func (e *fakeEnv) requestAt(i int) CloseRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i >= len(e.requests) {
		return CloseRequest{}
	}
	return e.requests[i]
}

// waitEvent 等 prompt 那个 goroutine 把事件推出来。
//
// prompt 是异步的（不阻塞窗口事件线程），断言前必须等它真的跑起来，
// 否则会读到「事件还没发」的中间态。
func (e *fakeEnv) waitEvent(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, n := e.counts(); n >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("等了 1s 也没等到第 %d 个事件", want)
}

func newTestGuard(t *testing.T) (*quitGuard, *fakeEnv) {
	t.Helper()
	env := &fakeEnv{}
	g := newQuitGuard(env.running, env.quit, env.emit)
	// 单测里把兜底超时调到几十毫秒，不用真等 5 秒。
	g.ackTimeout = 20 * time.Millisecond
	return g, env
}

func runningTask(projectID, taskID string) process.Runtime {
	return process.Runtime{ProjectID: projectID, TaskID: taskID, State: process.StateRunning}
}

// 没有任务在跑时，关闭必须原样放行，不能凭空弹框。
func TestQuitGuardLetsCloseThroughWhenIdle(t *testing.T) {
	g, env := newTestGuard(t)

	event := application.NewWindowEvent()
	g.HandleWindowClosing(event)

	if event.IsCancelled() {
		t.Fatal("没有任务在跑时不应该拦下关闭")
	}
	if quits, events := env.counts(); quits != 0 || events != 0 {
		t.Fatalf("没有任务在跑却产生了副作用：quits=%d events=%d", quits, events)
	}
}

// 有任务在跑：拦下关闭、把任务列表推给前端，并等用户决定。
func TestQuitGuardPromptsWhenTasksRunning(t *testing.T) {
	g, env := newTestGuard(t)
	env.setRunning(runningTask("p1", "web"), runningTask("p1", "api"))

	event := application.NewWindowEvent()
	g.HandleWindowClosing(event)

	if !event.IsCancelled() {
		t.Fatal("有任务在跑时必须先拦下关闭")
	}

	env.waitEvent(t, 1)
	req := env.requestAt(0)
	if len(req.Tasks) != 2 || req.Tasks[0].TaskID != "web" || req.Tasks[1].TaskID != "api" {
		t.Fatalf("推给前端的任务列表不对：%+v", req.Tasks)
	}

	g.AcknowledgeClose()
	// 回执之后就算用户慢慢想，也不能触发超时兜底退出。
	time.Sleep(3 * g.ackTimeout)
	if quits, _ := env.counts(); quits != 0 {
		t.Fatalf("前端已经回执，不该超时退出（quits=%d）", quits)
	}
}

// 前端没回执（webview 挂了 / 还没加载完）：不能把窗口锁死，按退出兜底。
func TestQuitGuardFallsBackWhenFrontendSilent(t *testing.T) {
	g, env := newTestGuard(t)
	env.setRunning(runningTask("p1", "web"))

	g.HandleWindowClosing(application.NewWindowEvent())
	env.waitEvent(t, 1)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if quits, _ := env.counts(); quits > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("前端没回执时应该兜底退出，否则窗口关不掉")
}

// 用户确认「停止并退出」：只退出一次，且之后的关闭不再拦截。
func TestQuitGuardConfirmQuitsOnce(t *testing.T) {
	g, env := newTestGuard(t)
	env.setRunning(runningTask("p1", "web"))

	g.HandleWindowClosing(application.NewWindowEvent())
	env.waitEvent(t, 1)

	g.ConfirmQuit()
	g.ConfirmQuit()

	if quits, _ := env.counts(); quits != 1 {
		t.Fatalf("确认退出应该只调一次 quit，实际 %d 次", quits)
	}

	// 退出流程里窗口还会再关一次，这时候必须放行。
	event := application.NewWindowEvent()
	g.HandleWindowClosing(event)
	if event.IsCancelled() {
		t.Fatal("确认退出后不应该再拦截关闭")
	}
}

// 用户取消：窗口留着，下次再点关闭要重新问一遍。
func TestQuitGuardCancelAllowsPromptingAgain(t *testing.T) {
	g, env := newTestGuard(t)
	env.setRunning(runningTask("p1", "web"))

	first := application.NewWindowEvent()
	g.HandleWindowClosing(first)
	if !first.IsCancelled() {
		t.Fatal("第一次关闭应该被拦下")
	}
	env.waitEvent(t, 1)
	g.AcknowledgeClose()
	g.CancelQuit()

	// 取消后不能因为旧 prompt 的超时兜底把应用退掉。
	time.Sleep(3 * g.ackTimeout)
	if quits, _ := env.counts(); quits != 0 {
		t.Fatalf("用户取消后不该退出（quits=%d）", quits)
	}

	second := application.NewWindowEvent()
	g.HandleWindowClosing(second)
	if !second.IsCancelled() {
		t.Fatal("再次关闭应该重新拦下")
	}
	env.waitEvent(t, 2)
}

// 确认框开着的时候再点关闭：继续拦着，但不能叠第二个框。
func TestQuitGuardDoesNotStackPrompts(t *testing.T) {
	g, env := newTestGuard(t)
	env.setRunning(runningTask("p1", "web"))

	g.HandleWindowClosing(application.NewWindowEvent())
	env.waitEvent(t, 1)

	second := application.NewWindowEvent()
	g.HandleWindowClosing(second)
	if !second.IsCancelled() {
		t.Fatal("确认框开着时，重复关闭仍然要被拦下")
	}

	time.Sleep(5 * time.Millisecond)
	if _, events := env.counts(); events != 1 {
		t.Fatalf("不应该弹出第二个确认框，实际发了 %d 个事件", events)
	}
}
