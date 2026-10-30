//go:build !windows

package prockill

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// startDetached 启动一个独立进程组里的 sleep，模拟「完全不受 bootpane
// 管的外部进程」——RestartTask 面对的就是这种。
func startDetached(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // 自成组长，pgid == pid
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 sleep 失败: %v", err)
	}
	return cmd
}

// TestTreeIncludesSelf 验证 Tree 返回的列表包含 root 自己——
// 接口注释承诺「返回 pid 及其所有子孙进程」。
func TestTreeIncludesSelf(t *testing.T) {
	cmd := startDetached(t)
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	victims, err := Tree(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Tree 失败: %v", err)
	}
	found := false
	for _, v := range victims {
		if v.PID == cmd.Process.Pid {
			found = true
		}
	}
	if !found {
		t.Fatalf("Tree(%d) 未包含 root 自己，返回：%+v", cmd.Process.Pid, victims)
	}
}

// TestKillStopsProcess 验证 Kill 真的能终止外部进程。
func TestKillStopsProcess(t *testing.T) {
	cmd := startDetached(t)

	killed, err := Kill(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Kill 失败: %v", err)
	}
	if len(killed) == 0 {
		t.Fatal("Kill 应至少终止 root 自己")
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Kill 后进程仍存活")
	}
}

// TestKillAlreadyExited 验证目标已退出时返回空列表不算错误
// ——重启场景下脚本可能刚好跑完。
func TestKillAlreadyExited(t *testing.T) {
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	_ = cmd.Wait() // 等它退干净

	killed, err := Kill(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("目标已退出时 Kill 不应报错: %v", err)
	}
	if len(killed) != 0 {
		t.Fatalf("目标已退出时应返回空列表，实际：%+v", killed)
	}
}
