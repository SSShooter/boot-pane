//go:build !windows

package proctree

import (
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"

	"bootpane/internal/pty"
)

// startGroup 用和 process.Manager 相同的链路启动一个会话：
// 建容器 → 起伪终端 → Attach 登记 → 起读泵。
// 读泵是 pty.Start 的调用契约（登录 shell 退出要等输出排空），必须挂。
func startGroup(t *testing.T, command string) (pty.Session, Group) {
	t.Helper()

	grp, err := New()
	if err != nil {
		t.Fatalf("创建进程组失败: %v", err)
	}
	sess, err := pty.Start(pty.Options{Command: command, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("启动会话失败: %v", err)
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := sess.Read(buf); err != nil {
				return
			}
		}
	}()
	grp.Attach(sess.Pid())
	return sess, grp
}

// TestUnixGroupKillTree 验证核心承诺：Kill 能把 shell 和它后台派生的
// 孙进程一起清掉。这是「pnpm → node → vite」场景的 Unix 等价物，
// 后台 sleep 逃不掉才算及格。
func TestUnixGroupKillTree(t *testing.T) {
	sess, grp := startGroup(t, "sleep 60 & sleep 60")
	defer func() {
		_ = sess.Close()
		_ = grp.Close()
	}()

	pgid := sess.Pid()

	// Kill 前进程组必须活着（kill 0 探测）。
	if err := syscall.Kill(-pgid, 0); err != nil {
		t.Fatalf("启动后进程组应存活: %v", err)
	}

	if err := grp.Kill(); err != nil {
		t.Fatalf("Kill 失败: %v", err)
	}

	code, err := sess.Wait()
	if err != nil && !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "signal") {
		t.Fatalf("等待退出出错: %v", err)
	}
	_ = code

	// 给内核一点收尸时间，然后确认整组都不在了。
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := syscall.Kill(-pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Kill 后进程组仍存活（kill(-%d,0) = %v）", pgid, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestUnixGroupKillIdempotent 验证 Kill 幂等：组都没了再 Kill 不报错。
func TestUnixGroupKillIdempotent(t *testing.T) {
	sess, grp := startGroup(t, "sleep 60")
	defer func() { _ = sess.Close() }()

	if err := grp.Kill(); err != nil {
		t.Fatalf("第一次 Kill 失败: %v", err)
	}
	_, _ = sess.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for {
		err := grp.Kill()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("第二次 Kill 应幂等返回 nil，实际: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestUnixGroupUsage 验证资源采样：组里至少有 shell 自己，进程数和
// 内存都应该大于 0。
func TestUnixGroupUsage(t *testing.T) {
	sess, grp := startGroup(t, "sleep 60")
	defer func() {
		_ = grp.Kill()
		_, _ = sess.Wait()
		_ = sess.Close()
		_ = grp.Close()
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		u, err := grp.Usage()
		if err != nil {
			t.Fatalf("采样失败: %v", err)
		}
		if u.Processes >= 1 && u.Memory > 0 {
			t.Logf("usage: processes=%d memory=%d cpu=%v", u.Processes, u.Memory, u.CPUTime)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("采样始终拿不到数据：%+v", u)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestUnixGroupAttachBeforeKill 验证未 Attach 时 Kill 是无害的空操作
// （New 和 Attach 之间崩掉的话，Shutdown 里的 Kill 不能炸）。
func TestUnixGroupAttachBeforeKill(t *testing.T) {
	grp, err := New()
	if err != nil {
		t.Fatalf("创建进程组失败: %v", err)
	}
	if err := grp.Kill(); err != nil {
		t.Fatalf("未 Attach 时 Kill 不应报错: %v", err)
	}
	u, err := grp.Usage()
	if err != nil {
		t.Fatalf("未 Attach 时 Usage 不应报错: %v", err)
	}
	if u.Processes != 0 {
		t.Fatalf("未 Attach 时进程数应为 0，实际 %d", u.Processes)
	}
}
