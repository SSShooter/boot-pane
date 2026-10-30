//go:build !windows

package pty

import (
	"strings"
	"testing"
	"time"
)

// drain 后台持续读会话，模拟 Manager 的日志泵。
//
// 这个动作是 Start 的调用契约（见 Start 注释）：登录 shell 退出时等
// 输出排空，master 无人读的话 Wait 会永远挂住。所有起会话的测试都要挂。
func drain(s Session) {
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := s.Read(buf); err != nil {
				return
			}
		}
	}()
}

// readUntil 从会话读到目标子串出现，或超时。
// 返回累计输出，方便失败时打全量日志定位。
func readUntil(t *testing.T, s Session, want string, timeout time.Duration) string {
	t.Helper()

	type result struct {
		data string
		err  error
	}
	ch := make(chan result, 1)

	var out strings.Builder
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
				if strings.Contains(out.String(), want) {
					ch <- result{data: out.String()}
					return
				}
			}
			if err != nil {
				ch <- result{data: out.String(), err: err}
				return
			}
		}
	}()

	select {
	case r := <-ch:
		if r.err != nil && !strings.Contains(r.data, want) {
			t.Fatalf("读会话出错（已有输出 %q）: %v", r.data, r.err)
		}
		// 目标子串已经出现，但队列里可能还有尾巴（比如结尾的 \r\n），
		// 继续在后台排空，否则 Wait 会卡在 shell 退出的 TCSADRAIN 上。
		drain(s)
		return r.data
	case <-time.After(timeout):
		t.Fatalf("等 %q 超时，已有输出：%q", want, out.String())
		return ""
	}
}

// TestStartEcho 验证伪终端链路：命令能跑、输出能收到（且经过 TTY，
// 大多数 shell/工具的路径都依赖这一点）、退出码能拿到。
func TestStartEcho(t *testing.T) {
	sess, err := Start(Options{Command: "echo hello-pty", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("启动会话失败: %v", err)
	}
	defer func() { _ = sess.Close() }()

	if sess.Pid() <= 0 {
		t.Fatalf("PID 异常：%d", sess.Pid())
	}

	readUntil(t, sess, "hello-pty", 10*time.Second)

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("等待退出出错: %v", err)
	}
	if code != 0 {
		t.Fatalf("退出码应为 0，实际 %d", code)
	}
}

// TestStartEnv 验证任务自带环境变量能传进 shell。
func TestStartEnv(t *testing.T) {
	sess, err := Start(Options{
		Command: "echo bootpane-env=$BOOTPANE_TEST_ENV",
		Env:     []string{"BOOTPANE_TEST_ENV=42", "PATH=/usr/bin:/bin"},
		Cols:    80,
		Rows:    24,
	})
	if err != nil {
		t.Fatalf("启动会话失败: %v", err)
	}
	defer func() { _ = sess.Close() }()

	readUntil(t, sess, "bootpane-env=42", 10*time.Second)
	_, _ = sess.Wait()
}

// TestResize 验证尺寸调整不报错（能不能真的生效归内核管）。
func TestResize(t *testing.T) {
	sess, err := Start(Options{Command: "sleep 1", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("启动会话失败: %v", err)
	}
	defer func() { _ = sess.Close() }()

	if err := sess.Resize(120, 40); err != nil {
		t.Fatalf("调整尺寸失败: %v", err)
	}
	_, _ = sess.Wait()
}

// TestStartNotFound 验证命令不存在的表现：shell 本身能启动（这点和
// Windows 的 CreateProcess 直接报错不同），命令执行失败后以 127 退出。
// Manager 靠「就绪前就退出」判定启动失败，127 正好落进那条路径。
func TestStartNotFound(t *testing.T) {
	sess, err := Start(Options{Command: "bootpane-definitely-not-exist-xyz", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("shell 本身应能启动: %v", err)
	}
	defer func() { _ = sess.Close() }()
	drain(sess)

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("等待退出出错: %v", err)
	}
	if code != 127 {
		t.Fatalf("命令不存在时退出码应为 127，实际 %d", code)
	}
}

// TestStartWithoutReader 用「无输出」命令验证不需要读也能拿到退出码；
// 同时注释说明：如果命令有输出又没人读，Wait 会卡在 shell 退出的
// TCSADRAIN 上——这是调用契约，不是 bug（见 Start 注释）。
func TestStartWithoutReader(t *testing.T) {
	sess, err := Start(Options{Command: "sleep 0.2", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("启动会话失败: %v", err)
	}
	defer func() { _ = sess.Close() }()

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("等待退出出错: %v", err)
	}
	if code != 0 {
		t.Fatalf("退出码应为 0，实际 %d", code)
	}
}
