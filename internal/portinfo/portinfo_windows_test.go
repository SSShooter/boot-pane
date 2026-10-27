//go:build windows

package portinfo

import (
	"net"
	"os"
	"strings"
	"testing"
)

// TestLookupListening 验证能查到自己在监听的端口，并反查到正确的 PID。
func TestLookupListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	got := Lookup(port)

	if !got.Listening {
		t.Fatalf("端口 %d 正在监听，应检测到", port)
	}
	if got.PID != os.Getpid() {
		t.Fatalf("PID 应为 %d（本测试进程），实际 %d", os.Getpid(), got.PID)
	}
	// 进程名要真的指向测试二进制，才能证明 QueryFullProcessImageName 走通了，
	// 而不是返回了空串蒙混过关。
	if !strings.Contains(strings.ToLower(got.Process), ".test") {
		t.Fatalf("进程名应指向测试二进制，实际 %q", got.Process)
	}
}

// TestLookupIPv6 验证 IPv6 监听也能查到。
//
// 单独测是因为 node / vite 默认监听 `::`，只查 IPv4 表会漏掉一大半前端项目。
func TestLookupIPv6(t *testing.T) {
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("本机不支持 IPv6: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	got := Lookup(port)

	if !got.Listening {
		t.Fatalf("IPv6 端口 %d 正在监听，应检测到", port)
	}
	if got.PID != os.Getpid() {
		t.Fatalf("PID 应为 %d，实际 %d", os.Getpid(), got.PID)
	}
}

// TestLookupFree 验证没人监听的端口返回 Listening=false。
func TestLookupFree(t *testing.T) {
	// 先占一个端口再释放，拿到一个确定空闲的端口号。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	got := Lookup(port)
	if got.Listening {
		t.Fatalf("端口 %d 已释放，不该被判定为监听中（PID %d）", port, got.PID)
	}
	if got.PID != 0 || got.Process != "" {
		t.Fatalf("空闲端口不该带进程信息，实际 %+v", got)
	}
}

// TestLookupAllDedup 验证批量查询会去重并保持顺序。
func TestLookupAllDedup(t *testing.T) {
	got := LookupAll([]int{12345, 12345, 0, -1, 70000, 23456})
	if len(got) != 2 {
		t.Fatalf("应只剩 2 个合法且不重复的端口，实际 %d 个: %+v", len(got), got)
	}
	if got[0].Port != 12345 || got[1].Port != 23456 {
		t.Fatalf("顺序应与入参一致，实际 %+v", got)
	}
}
