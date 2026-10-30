//go:build !windows

package portinfo

import (
	"net"
	"os"
	"testing"
)

// TestLookupSelfListener 验证 lsof 链路：本测试进程监听一个端口后，
// Lookup 应能查到 Listening=true 且 PID 是自己。
func TestLookupSelfListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起监听失败: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	got := Lookup(port)
	if !got.Listening {
		t.Fatalf("端口 %d 正在监听，Lookup 却说没有（lsof 输出解析或执行有问题）", port)
	}
	if got.PID != os.Getpid() {
		t.Fatalf("监听者应为本进程（pid=%d），实际 %+v", os.Getpid(), got)
	}
}

// TestLookupFreePort 验证没人监听的端口返回 Listening=false。
func TestLookupFreePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // 立刻释放，拿到一个当前没人用的端口

	got := Lookup(port)
	if got.Listening {
		t.Fatalf("端口 %d 已释放，Lookup 却说有监听者：%+v", port, got)
	}
}
