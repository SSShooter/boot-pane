//go:build windows

package prockill

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestKillTree 验证能连同子进程一起清掉。
//
// 用 `cmd /c ping` 构造两层结构：cmd 会派生 ping，只杀 cmd 的话 ping 会变成
// 孤儿继续跑——这正是重启外部实例时最怕的情况。
func TestKillTree(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping -n 30 127.0.0.1 >nul")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试进程失败: %v", err)
	}
	rootPID := cmd.Process.Pid

	// 兜底：测试失败也要把进程清掉，别留垃圾。
	t.Cleanup(func() {
		_, _ = Kill(rootPID)
		_ = cmd.Wait()
	})

	// 等 ping 派生出来。
	time.Sleep(700 * time.Millisecond)

	targets, err := Tree(rootPID)
	if err != nil {
		t.Fatalf("遍历进程树失败: %v", err)
	}
	if len(targets) < 2 {
		t.Fatalf("应至少包含 cmd 和它派生的 ping，实际 %d 个: %+v", len(targets), targets)
	}
	// 子进程必须排在父进程前面，否则先杀父会让子进程逃逸。
	if targets[len(targets)-1].PID != rootPID {
		t.Fatalf("根进程应排在最后，实际 %+v", targets)
	}

	victims, err := Kill(rootPID)
	if err != nil {
		t.Fatalf("终止进程树失败: %v", err)
	}
	if len(victims) < 2 {
		t.Fatalf("应终止 cmd 和 ping 两个进程，实际 %+v", victims)
	}

	// Wait 能返回说明根进程确实死了。
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("根进程在 5s 内没有退出")
	}
}

// TestKillSelf 验证不会把 bootpane 自己杀掉。
func TestKillSelf(t *testing.T) {
	if _, err := Kill(os.Getpid()); err == nil {
		t.Fatal("终止自身必须被拒绝")
	}
}

// TestTreeMissingPID 验证不存在的 PID 返回错误而不是空列表。
func TestTreeMissingPID(t *testing.T) {
	// 0x7FFFFFF0 这个量级的 PID 在 Windows 上不会出现。
	if _, err := Tree(0x7FFFFFF0); err == nil {
		t.Fatal("不存在的进程应返回错误")
	}
}

// TestKillInvalidPID 验证非法 PID 被挡下。
func TestKillInvalidPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if _, err := Kill(pid); err == nil {
			t.Fatalf("PID %d 应被拒绝", pid)
		}
	}
}
