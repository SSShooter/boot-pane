//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"bootpane/internal/config"
	"bootpane/internal/portinfo"
	"bootpane/internal/process"
)

// listenerScript 是一个占住端口的假服务，模拟「用户自己起的、bootpane 管不到」的实例。
const listenerScript = `param([int]$Port)
$l = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
$l.Start()
Write-Output "listening on $Port"
Start-Sleep -Seconds 60
`

// freePort 借一个空闲端口号。
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("分配空闲端口失败: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// waitListening 轮询到端口进入监听状态。
func waitListening(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("端口 %d 在 %v 内没有开始监听", port, timeout)
}

// TestRestartTaskReplacesExternalInstance 覆盖重启外部实例的完整链路：
// 起一个不受管的进程占住端口 → RestartTask 杀掉它 → 等端口释放 → 启动自己的实例。
func TestRestartTaskReplacesExternalInstance(t *testing.T) {
	if testing.Short() {
		t.Skip("集成测试，-short 下跳过")
	}

	projectDir := t.TempDir()
	scriptPath := filepath.Join(projectDir, "listener.ps1")
	if err := os.WriteFile(scriptPath, []byte(listenerScript), 0o644); err != nil {
		t.Fatalf("写测试脚本失败: %v", err)
	}

	port := freePort(t)

	// 1. 先起一个「外部实例」，完全不受 bootpane 管。
	external := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath, "-Port", strconv.Itoa(port))
	external.Dir = projectDir
	if err := external.Start(); err != nil {
		t.Fatalf("启动外部实例失败: %v", err)
	}
	t.Cleanup(func() {
		_ = external.Process.Kill()
		_ = external.Wait()
	})

	waitListening(t, port, 20*time.Second)

	if l := portinfo.Lookup(port); !l.Listening {
		t.Fatalf("端口 %d 正在监听，应被识别为外部占用", port)
	}

	// 2. 注册项目和任务，任务命令就是同一个监听脚本。
	store, err := config.NewStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}
	proj, err := store.Add(projectDir)
	if err != nil {
		t.Fatalf("添加项目失败: %v", err)
	}
	tasks := []config.Task{{
		ID:      "svc",
		Label:   "假服务",
		Command: fmt.Sprintf("powershell -NoProfile -ExecutionPolicy Bypass -File listener.ps1 -Port %d", port),
		WaitFor: &config.WaitFor{Type: config.WaitPort, Port: port, Timeout: 30},
	}}
	if err := store.SetTasks(proj.ID, tasks, nil); err != nil {
		t.Fatalf("写入任务失败: %v", err)
	}

	mgr := process.NewManager(nil)
	defer mgr.Shutdown()
	svc := NewProcessService(mgr, store)

	// 3. 重启。
	res := svc.RestartTask(proj.ID, "svc")
	if !res.OK {
		t.Fatalf("重启失败: %s", res.Message)
	}
	if len(res.Killed) == 0 {
		t.Fatal("应记录被终止的外部进程")
	}

	// 4. 外部实例必须真的死了。
	done := make(chan error, 1)
	go func() { done <- external.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("外部实例在 10s 内没有退出")
	}

	// 5. 新实例要跑起来并重新占住端口。
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if mgr.Get(proj.ID, "svc").State == process.StateRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	rt := mgr.Get(proj.ID, "svc")
	if rt.State != process.StateRunning {
		t.Fatalf("新实例应就绪，实际状态 %s（%s）", rt.State, rt.LastError)
	}
	if rt.PID == 0 {
		t.Fatal("新实例的 PID 不应为 0")
	}
}

// TestRestartTaskWithoutPort 验证没配端口的任务会被明确拒绝。
func TestRestartTaskWithoutPort(t *testing.T) {
	projectDir := t.TempDir()
	store, err := config.NewStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}
	proj, err := store.Add(projectDir)
	if err != nil {
		t.Fatalf("添加项目失败: %v", err)
	}
	if err := store.SetTasks(proj.ID, []config.Task{{
		ID:      "build",
		Label:   "编译",
		Command: "echo hi",
		WaitFor: &config.WaitFor{Type: config.WaitLog, Log: "done"},
	}}, nil); err != nil {
		t.Fatalf("写入任务失败: %v", err)
	}

	mgr := process.NewManager(nil)
	defer mgr.Shutdown()
	svc := NewProcessService(mgr, store)

	res := svc.RestartTask(proj.ID, "build")
	if res.OK {
		t.Fatal("没有端口的任务不该允许重启")
	}
	if res.Message == "" {
		t.Fatal("应给出拒绝原因")
	}
}
