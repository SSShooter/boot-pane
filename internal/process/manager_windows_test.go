//go:build windows

package process

import (
	"strings"
	"sync"
	"testing"
	"time"

	"bootpane/internal/config"
	"bootpane/internal/i18n"
)

// eventRecorder 收集推给前端的事件，供断言使用。
type eventRecorder struct {
	mu     sync.Mutex
	logs   []string
	states []string
	stats  int
}

func (r *eventRecorder) emit(event string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch event {
	case EventLog:
		r.logs = append(r.logs, data.(LogEvent).Chunk)
	case EventRuntime:
		r.states = append(r.states, data.(Runtime).State)
	case EventStats:
		r.stats++
	}
}

func (r *eventRecorder) logText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.logs, "")
}

// waitState 轮询等待任务进入目标状态。
func waitState(t *testing.T, m *Manager, state string, timeout time.Duration) Runtime {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rt := m.Get("p1", "t1")
		if rt.State == state {
			return rt
		}
		time.Sleep(50 * time.Millisecond)
	}
	rt := m.Get("p1", "t1")
	t.Fatalf("等待状态 %q 超时，当前 %q（lastError=%s）", state, rt.State, rt.LastError)
	return rt
}

// TestManagerLifecycle 覆盖完整链路：启动 → 日志就绪判定 → running → 自然退出 → stopped。
func TestManagerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试，-short 下跳过")
	}

	rec := &eventRecorder{}
	m := NewManager(rec.emit)
	defer m.Shutdown()

	task := config.Task{
		ID:      "t1",
		Label:   "生命周期",
		Command: "echo ready-marker && ping -n 3 127.0.0.1 >nul",
		WaitFor: &config.WaitFor{Type: config.WaitLog, Log: "ready-marker", Timeout: 15},
	}

	if err := m.Start("p1", task, t.TempDir()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	running := waitState(t, m, StateRunning, 15*time.Second)
	if running.PID == 0 {
		t.Fatal("就绪后 PID 不应为 0")
	}
	if !running.Ready {
		t.Fatal("running 状态下 Ready 应为 true")
	}

	stopped := waitState(t, m, StateStopped, 20*time.Second)
	if stopped.ExitCode != 0 {
		t.Fatalf("退出码应为 0，实际 %d", stopped.ExitCode)
	}
	// 进程退出后资源占用必须清零，否则界面会一直显示死进程的数字。
	if stopped.CPUPercent != 0 || stopped.MemoryBytes != 0 {
		t.Fatalf("退出后资源应清零，实际 cpu=%v mem=%d", stopped.CPUPercent, stopped.MemoryBytes)
	}

	if got := rec.logText(); !strings.Contains(got, "ready-marker") {
		t.Fatalf("日志事件里应含 ready-marker，实际 %q", got)
	}
	if got := m.Log("p1", "t1"); !strings.Contains(got, "ready-marker") {
		t.Fatalf("环形缓冲应保留日志，实际 %q", got)
	}
}

// TestManagerStopKillsTree 验证 Stop 能把整棵进程树清干净。
//
// 这里刻意用 ping 制造一个「父进程会派生子进程」的场景：
// 如果只杀 cmd 而不杀整棵树，ping 会活下来继续占资源。
func TestManagerStopKillsTree(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试，-short 下跳过")
	}

	m := NewManager(nil)
	defer m.Shutdown()

	task := config.Task{
		ID:      "t1",
		Label:   "长时间运行",
		Command: "ping -n 60 127.0.0.1 >nul",
	}

	if err := m.Start("p1", task, t.TempDir()); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	// 没配 waitFor，进程起来即 running。
	waitState(t, m, StateRunning, 10*time.Second)

	// 等采样跑一轮，确认能拿到进程树数据。
	deadline := time.Now().Add(5 * time.Second)
	var sawStats bool
	for time.Now().Before(deadline) {
		if rt := m.Get("p1", "t1"); rt.Processes > 0 && rt.MemoryBytes > 0 {
			sawStats = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !sawStats {
		t.Fatalf("应能采到进程树资源，实际 %+v", m.Get("p1", "t1"))
	}

	start := time.Now()
	if err := m.Stop("p1", "t1"); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	stopped := waitState(t, m, StateStopped, 10*time.Second)

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("停止耗时过长: %v", elapsed)
	}
	// 主动停止不算失败。
	if stopped.LastError != "" {
		t.Fatalf("主动停止不应产生错误信息，实际 %q", stopped.LastError)
	}
}

// TestManagerStartFailsOnMissingDir 验证工作目录不存在时不会真的去起进程。
func TestManagerStartFailsOnMissingDir(t *testing.T) {
	// 错误文案跟着应用语言变，这里断言的是「报的是目录问题」，固定成中文。
	orig := i18n.Current()
	i18n.Set(string(i18n.ZH))
	t.Cleanup(func() { i18n.Set(string(orig)) })

	m := NewManager(nil)
	defer m.Shutdown()

	task := config.Task{
		ID:      "t1",
		Label:   "缺目录",
		Command: "echo hi",
		Cwd:     "not-there",
	}
	if err := m.Start("p1", task, t.TempDir()); err != nil {
		t.Fatalf("Start 本身不该返回 error，应通过状态表达失败: %v", err)
	}

	rt := m.Get("p1", "t1")
	if rt.State != StateFailed {
		t.Fatalf("状态应为 failed，实际 %s", rt.State)
	}
	if !strings.Contains(rt.LastError, "工作目录不存在") {
		t.Fatalf("错误信息应说明工作目录问题，实际 %q", rt.LastError)
	}
}
