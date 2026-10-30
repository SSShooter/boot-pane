// Package process 负责启动任务、跟踪状态、以及把输出推给前端。
//
// 状态机是 UI 的唯一数据来源：
//
//	idle → starting → running → stopped
//	                 ↘ failed
//
// starting 不等于 running：只有 WaitFor 条件真正满足（端口在监听 / 日志出现关键字）
// 才算就绪，按钮才允许从 loading 变成运行中。前端不允许自己猜状态。
package process

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"bootpane/internal/config"
	"bootpane/internal/proctree"
	"bootpane/internal/pty"
)

// 推给前端的事件名。改动要同步 frontend/src/lib/events.ts。
const (
	EventRuntime = "bootpane:runtime" // 状态快照变化
	EventLog     = "bootpane:log"     // 日志增量（已节流）
	EventStats   = "bootpane:stats"   // CPU / 内存采样
)

// 任务运行时状态。
const (
	StateIdle     = "idle"
	StateStarting = "starting"
	StateRunning  = "running"
	StateFailed   = "failed"
	StateStopped  = "stopped"
)

const (
	// 日志攒批的触发条件：攒够 50ms 或 32KB 就推一次。
	// 不节流的话 vite 启动瞬间的几千行输出会把 Wails 事件通道打满，
	// 前端卡到连按钮都点不动。
	logFlushInterval = 50 * time.Millisecond
	logFlushBytes    = 32 * 1024

	// 资源采样周期。1s 足够让用户看清趋势，又不至于让 Job 查询成为负担。
	statsInterval = time.Second

	// 会话关闭前等日志泵收尾的上限。
	pumpDrainTimeout = 2 * time.Second
)

// ErrNotImplemented 保留给尚未接入的平台实现。
var ErrNotImplemented = errors.New("进程管理尚未接入")

// Runtime 是一个任务当前的运行快照。
type Runtime struct {
	ProjectID   string    `json:"projectId"`
	TaskID      string    `json:"taskId"`
	State       string    `json:"state"`
	PID         int       `json:"pid"`
	Ready       bool      `json:"ready"` // WaitFor 是否已满足
	StartedAt   time.Time `json:"startedAt"`
	ExitCode    int       `json:"exitCode"`
	LastError   string    `json:"lastError,omitempty"`
	CPUPercent  float64   `json:"cpuPercent"`  // 整棵进程树，按全部核心归一化
	MemoryBytes uint64    `json:"memoryBytes"` // 整棵进程树工作集之和
	Processes   int       `json:"processes"`   // 当前活着的进程数
}

// Key 是任务在管理器里的唯一键。
func (r Runtime) Key() string { return r.ProjectID + "/" + r.TaskID }

// LogEvent 是日志增量事件。
type LogEvent struct {
	ProjectID string `json:"projectId"`
	TaskID    string `json:"taskId"`
	Chunk     string `json:"chunk"`
}

// StatsEvent 是一次资源采样事件。
type StatsEvent struct {
	ProjectID   string  `json:"projectId"`
	TaskID      string  `json:"taskId"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryBytes uint64  `json:"memoryBytes"`
	Processes   int     `json:"processes"`
}

// Emitter 把事件推给前端。用函数注入而不是直接依赖 Wails 的 app 实例，
// 这样这个包能脱离桌面壳单测。
type Emitter func(event string, data any)

// taskRun 是单个任务的内部状态，Runtime 部分是给前端看的切片。
type taskRun struct {
	Runtime

	group proctree.Group
	sess  pty.Session
	logs  *logRing

	// stopping 标记「是用户主动停的」。进程退出时要靠它区分
	// stopped（正常停止）和 failed（自己崩了）。
	stopping bool

	readyCh   chan struct{}
	readyOnce sync.Once

	// pumpDone 在日志泵退出后关闭，清理时靠它确认没有 goroutine 还在 Read。
	pumpDone chan struct{}

	// CPU 百分比是差分量，必须记住上一次采样的累计值和时刻。
	cpuPrev time.Duration
	cpuAt   time.Time
}

// Manager 持有所有正在运行的任务。
type Manager struct {
	mu   sync.RWMutex
	runs map[string]*taskRun

	emit   Emitter
	stopCh chan struct{}
	closed sync.Once
}

// NewManager 创建进程管理器。emit 为 nil 时事件会被丢弃（单测场景）。
func NewManager(emit Emitter) *Manager {
	if emit == nil {
		emit = func(string, any) {}
	}
	m := &Manager{
		runs:   make(map[string]*taskRun),
		emit:   emit,
		stopCh: make(chan struct{}),
	}
	go m.sampleLoop()
	return m
}

// Start 启动一个任务。
//
// 完整链路：建进程树容器 → 用伪终端启动 shell（Windows 在同一次
// CreateProcess 里挂上 Job Object；Unix 由 Setsid 定好进程组后 Attach 登记）
// → 起日志泵 → 等 WaitFor 就绪 → 进程退出时收尾。
//
// 返回 nil 不代表启动成功，只代表「已经开始尝试」。真正的成败通过事件推给前端，
// 因为就绪判定是异步的，同步等它会把 UI 卡住。
func (m *Manager) Start(projectID string, task config.Task, root string) error {
	key := projectID + "/" + task.ID

	m.mu.Lock()
	if old, ok := m.runs[key]; ok {
		if task.SingletonEnabled() && (old.State == StateStarting || old.State == StateRunning) {
			m.mu.Unlock()
			return fmt.Errorf("「%s」已经在运行中", task.Label)
		}
	}
	r := &taskRun{
		Runtime: Runtime{
			ProjectID: projectID,
			TaskID:    task.ID,
			State:     StateStarting,
			StartedAt: time.Now(),
		},
		logs:     newLogRing(defaultLogBytes),
		readyCh:  make(chan struct{}),
		pumpDone: make(chan struct{}),
	}
	m.runs[key] = r
	m.mu.Unlock()
	m.emitRuntime(key)

	// 工作目录：cwd 是相对项目根目录的，空或 "." 表示就在根目录。
	dir := root
	if task.Cwd != "" && task.Cwd != "." {
		dir = filepath.Join(root, task.Cwd)
	}
	if fi, statErr := os.Stat(dir); statErr != nil || !fi.IsDir() {
		m.fail(key, fmt.Sprintf("工作目录不存在：%s", dir))
		return nil
	}

	grp, err := proctree.New()
	if err != nil {
		m.fail(key, "创建进程树失败："+err.Error())
		return nil
	}

	sess, err := pty.Start(pty.Options{
		Command:   task.Command,
		Dir:       dir,
		Env:       mergeEnv(os.Environ(), task.Env),
		Cols:      120,
		Rows:      30,
		JobHandle: grp.Handle(),
	})
	if err != nil {
		_ = grp.Close()
		m.fail(key, "启动失败："+err.Error())
		return nil
	}

	// Unix 在这里登记 pgid（== shell PID，启动时 Setsid 定好的）；
	// Windows 上进程出生即在 Job 里，Attach 是空操作。
	// 必须在把 grp 暴露给 Stop / Shutdown 之前完成，否则存在「任务刚启动
	// 就被停掉，但容器还不知道组 ID」的竞态窗口。
	grp.Attach(sess.Pid())

	m.mu.Lock()
	r.group = grp
	r.sess = sess
	r.PID = sess.Pid()
	m.mu.Unlock()
	m.emitRuntime(key)

	go m.pumpLogs(r, sess, task.WaitFor)
	go m.awaitReady(key, r, task.WaitFor)
	go m.waitExit(key, r, sess, grp)

	return nil
}

// Stop 终止任务及其整棵进程树。任务本来就没在跑时返回 nil，保持幂等。
func (m *Manager) Stop(projectID, taskID string) error {
	key := projectID + "/" + taskID

	m.mu.Lock()
	r, ok := m.runs[key]
	if !ok || r.group == nil {
		m.mu.Unlock()
		return nil
	}
	// 先打标记再杀，否则 waitExit 可能先看到进程死了、误判成「自己崩了」。
	r.stopping = true
	grp := r.group
	m.mu.Unlock()

	return grp.Kill()
}

// StopProject 终止某个项目下所有任务。
func (m *Manager) StopProject(projectID string) error {
	m.mu.RLock()
	type item struct{ projectID, taskID string }
	items := make([]item, 0, len(m.runs))
	for _, r := range m.runs {
		if r.ProjectID == projectID && r.group != nil {
			items = append(items, item{r.ProjectID, r.TaskID})
		}
	}
	m.mu.RUnlock()

	var firstErr error
	for _, it := range items {
		if err := m.Stop(it.projectID, it.taskID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Running 返回所有正在运行（含正在启动）的任务快照。
//
// 关窗口前用它判断要不要拦下来问一句：stopped / failed 的任务进程树
// 已经收尾，不需要再提示。
func (m *Manager) Running() []Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Runtime, 0, len(m.runs))
	for _, r := range m.runs {
		if r.State == StateRunning || r.State == StateStarting {
			out = append(out, r.Runtime)
		}
	}
	return out
}

// List 返回所有任务的运行快照。
func (m *Manager) List() []Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Runtime, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, r.Runtime)
	}
	return out
}

// Get 返回单个任务的快照，不存在时返回 idle 状态。
func (m *Manager) Get(projectID, taskID string) Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.runs[projectID+"/"+taskID]; ok {
		return r.Runtime
	}
	return Runtime{ProjectID: projectID, TaskID: taskID, State: StateIdle}
}

// Log 返回某个任务已缓冲的日志全量，用于前端首次打开面板。
func (m *Manager) Log(projectID, taskID string) string {
	m.mu.RLock()
	r, ok := m.runs[projectID+"/"+taskID]
	m.mu.RUnlock()
	if !ok {
		return ""
	}
	return r.logs.String()
}

// Shutdown 停掉所有任务和采样循环，应用退出时调用。
func (m *Manager) Shutdown() {
	m.closed.Do(func() { close(m.stopCh) })

	m.mu.RLock()
	groups := make([]proctree.Group, 0, len(m.runs))
	for _, r := range m.runs {
		if r.group != nil {
			groups = append(groups, r.group)
		}
	}
	m.mu.RUnlock()

	for _, g := range groups {
		_ = g.Kill()
	}
}

// pumpLogs 读终端输出，节流后推给前端，同时负责 log 类型的就绪判定。
func (m *Manager) pumpLogs(r *taskRun, sess pty.Session, wf *config.WaitFor) {
	defer close(r.pumpDone)

	var matcher *logMatcher
	if wf != nil && wf.Type == config.WaitLog && wf.Log != "" {
		matcher = newLogMatcher(wf.Log)
	}

	// 读 goroutine：Read 是阻塞的，塞进 channel 后由下面的 select 统一调度。
	dataCh := make(chan []byte, 32)
	go func() {
		defer close(dataCh)
		buf := make([]byte, 32*1024)
		for {
			n, err := sess.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				// channel 满时这里会阻塞，形成背压——这是刻意的，
				// 宁可让子进程写慢一点，也不能丢日志。
				dataCh <- chunk
			}
			if err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()

	pending := make([]byte, 0, logFlushBytes)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		chunk := string(pending)
		pending = pending[:0]

		r.logs.append(chunk)
		m.emit(EventLog, LogEvent{ProjectID: r.ProjectID, TaskID: r.TaskID, Chunk: chunk})

		if matcher != nil && matcher.feed(chunk) {
			r.readyOnce.Do(func() { close(r.readyCh) })
		}
	}

	for {
		select {
		case b, ok := <-dataCh:
			if !ok {
				flush()
				return
			}
			pending = append(pending, b...)
			if len(pending) >= logFlushBytes {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// awaitReady 等 WaitFor 条件满足，然后把状态推进到 running。
func (m *Manager) awaitReady(key string, r *taskRun, wf *config.WaitFor) {
	if wf == nil {
		// 没配 waitFor 就没有判据，只能认为「进程起来了就算跑着」。
		m.setState(key, StateRunning, true)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), resolveWaitTimeout(wf))
	defer cancel()

	var err error
	switch wf.Type {
	case config.WaitPort:
		err = waitPort(ctx, wf.Port)
	case config.WaitLog:
		select {
		case <-r.readyCh:
		case <-ctx.Done():
			err = fmt.Errorf("等待日志 %q 超时", wf.Log)
		}
	case "":
		// 配了 waitFor 但没写 type，当作没配。
		m.setState(key, StateRunning, true)
		return
	default:
		err = fmt.Errorf("未知的 waitFor 类型：%s", wf.Type)
	}

	if err != nil {
		m.fail(key, err.Error())
		return
	}
	m.setState(key, StateRunning, true)
}

// waitExit 等进程退出并收尾。
func (m *Manager) waitExit(key string, r *taskRun, sess pty.Session, grp proctree.Group) {
	code, waitErr := sess.Wait()

	// 等日志泵把最后一段输出冲完再关会话。顺序反了的话，还在 ReadFile
	// 的 goroutine 会拿到一个已被关闭的句柄。
	select {
	case <-r.pumpDone:
	case <-time.After(pumpDrainTimeout):
	}

	m.mu.Lock()
	r.ExitCode = code
	stopping := r.stopping
	wasStarting := r.State == StateStarting

	switch {
	case stopping:
		r.State = StateStopped
	case wasStarting:
		// 还没就绪就退出了，这是启动失败而不是正常结束。
		r.State = StateFailed
		if r.LastError == "" {
			if waitErr != nil {
				r.LastError = waitErr.Error()
			} else {
				r.LastError = fmt.Sprintf("进程启动后立即退出（退出码 %d）", code)
			}
		}
	default:
		r.State = StateStopped
	}

	// 资源占用清零，否则前端会一直显示退出前的数字。
	r.Ready = false
	r.CPUPercent = 0
	r.MemoryBytes = 0
	r.Processes = 0
	m.mu.Unlock()

	_ = sess.Close()
	_ = grp.Close()

	m.emitRuntime(key)
}

// sampleLoop 周期性采集所有运行中任务的 CPU / 内存。
func (m *Manager) sampleLoop() {
	ticker := time.NewTicker(statsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sample()
		}
	}
}

func (m *Manager) sample() {
	type target struct {
		key   string
		group proctree.Group
		prev  time.Duration
		at    time.Time
	}

	m.mu.RLock()
	targets := make([]target, 0, len(m.runs))
	for k, r := range m.runs {
		if r.group == nil || (r.State != StateRunning && r.State != StateStarting) {
			continue
		}
		targets = append(targets, target{key: k, group: r.group, prev: r.cpuPrev, at: r.cpuAt})
	}
	m.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	now := time.Now()
	cores := float64(runtime.NumCPU())

	for _, t := range targets {
		usage, err := t.group.Usage()
		if err != nil {
			continue
		}

		var pct float64
		if !t.at.IsZero() {
			elapsed := now.Sub(t.at).Seconds()
			// Job 的 CPU 时间是累计值，必须用两次采样的差除以时间间隔。
			// 除以核心数是为了跟任务管理器同口径，否则多核机器上会出现 300%。
			if elapsed > 0 && usage.CPUTime > t.prev {
				pct = float64(usage.CPUTime-t.prev) / float64(time.Second) / elapsed / cores * 100
			}
		}
		if pct > 100 {
			pct = 100
		}
		pct = math.Round(pct*10) / 10

		m.mu.Lock()
		r, ok := m.runs[t.key]
		if !ok {
			m.mu.Unlock()
			continue
		}
		r.cpuPrev = usage.CPUTime
		r.cpuAt = now
		r.CPUPercent = pct
		r.MemoryBytes = usage.Memory
		r.Processes = usage.Processes
		ev := StatsEvent{
			ProjectID:   r.ProjectID,
			TaskID:      r.TaskID,
			CPUPercent:  pct,
			MemoryBytes: usage.Memory,
			Processes:   usage.Processes,
		}
		m.mu.Unlock()

		m.emit(EventStats, ev)
	}
}

// setState 推进状态。
//
// 已经终止的任务不允许被迟到的就绪信号拉回 running：进程秒退时，
// awaitReady 的端口轮询可能还在跑，超时前刚好有个别的进程占了那个端口，
// 就会把 failed 覆盖成 running。
func (m *Manager) setState(key, state string, ready bool) {
	m.mu.Lock()
	r, ok := m.runs[key]
	if !ok || r.State == StateStopped || r.State == StateFailed {
		m.mu.Unlock()
		return
	}
	r.State = state
	r.Ready = ready
	m.mu.Unlock()

	m.emitRuntime(key)
}

// fail 把任务标成失败并记录原因。
func (m *Manager) fail(key, msg string) {
	m.mu.Lock()
	if r, ok := m.runs[key]; ok {
		r.State = StateFailed
		r.Ready = false
		r.LastError = msg
	}
	m.mu.Unlock()

	m.emitRuntime(key)
}

func (m *Manager) emitRuntime(key string) {
	m.mu.RLock()
	r, ok := m.runs[key]
	var snapshot Runtime
	if ok {
		snapshot = r.Runtime
	}
	m.mu.RUnlock()

	if ok {
		m.emit(EventRuntime, snapshot)
	}
}

// mergeEnv 把任务自带的 env 叠加到父进程环境上，同名覆盖。
func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}

	overridden := make(map[string]bool, len(extra))
	for k := range extra {
		overridden[strings.ToUpper(k)] = true
	}

	out := make([]string, 0, len(base)+len(extra))
	for _, kv := range base {
		if idx := strings.IndexByte(kv, '='); idx > 0 && overridden[strings.ToUpper(kv[:idx])] {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}
