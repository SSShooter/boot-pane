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
	"errors"
	"sync"
	"time"

	"bootpane/internal/config"
)

// ErrNotImplemented 表示进程管理尚未接入（M1 实现）。
var ErrNotImplemented = errors.New("进程管理尚未接入")

// 任务运行时状态。
const (
	StateIdle     = "idle"
	StateStarting = "starting"
	StateRunning  = "running"
	StateFailed   = "failed"
	StateStopped  = "stopped"
)

// Runtime 是一个任务当前的运行快照。
type Runtime struct {
	ProjectID string    `json:"projectId"`
	TaskID    string    `json:"taskId"`
	State     string    `json:"state"`
	PID       int       `json:"pid"`
	Ready     bool      `json:"ready"` // WaitFor 是否已满足
	StartedAt time.Time `json:"startedAt"`
	ExitCode  int       `json:"exitCode"`
	LastError string    `json:"lastError,omitempty"`
}

// Key 是任务在管理器里的唯一键。
func (r Runtime) Key() string { return r.ProjectID + "/" + r.TaskID }

// Manager 持有所有正在运行的任务。
type Manager struct {
	mu   sync.RWMutex
	runs map[string]*Runtime
}

// NewManager 创建进程管理器。
func NewManager() *Manager {
	return &Manager{runs: make(map[string]*Runtime)}
}

// Start 启动一个任务。
//
// M1 阶段要在这里串起完整链路：proctree.Attach → pty.Start → 读流推事件 →
// WaitFor 就绪判定 → 状态迁移。当前只留接口。
func (m *Manager) Start(projectID string, task config.Task, root string) error {
	return ErrNotImplemented
}

// Stop 终止任务及其整棵进程树。任务本来就没在跑时返回 nil，保持幂等。
func (m *Manager) Stop(projectID, taskID string) error {
	return nil
}

// StopProject 终止某个项目下所有任务。
func (m *Manager) StopProject(projectID string) error {
	m.mu.RLock()
	keys := make([]string, 0, len(m.runs))
	for k, r := range m.runs {
		if r.ProjectID == projectID {
			keys = append(keys, k)
		}
	}
	m.mu.RUnlock()

	for _, k := range keys {
		r := m.runs[k]
		if err := m.Stop(r.ProjectID, r.TaskID); err != nil {
			return err
		}
	}
	return nil
}

// List 返回所有任务的运行快照。
func (m *Manager) List() []Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Runtime, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, *r)
	}
	return out
}

// Get 返回单个任务的快照，不存在时返回 idle 状态。
func (m *Manager) Get(projectID, taskID string) Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.runs[projectID+"/"+taskID]; ok {
		return *r
	}
	return Runtime{ProjectID: projectID, TaskID: taskID, State: StateIdle}
}
