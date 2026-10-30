package process

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"bootpane/internal/config"
)

const (
	// 端口轮询间隔。100ms 是「点完按钮很快就亮」和轮询开销之间的折中。
	portPollInterval = 100 * time.Millisecond

	// 单次探测超时。探测的是本机端口，超过这个时间基本就是没监听。
	portDialTimeout = 300 * time.Millisecond

	// 没配 timeout 时的默认上限。vite 冷启动一般 3-10 秒，
	// 首次 pnpm install 那种场景用户自己会把 timeout 调大。
	defaultWaitTimeout = 60 * time.Second
)

// waitPort 轮询直到端口进入监听状态。
//
// 不用一次 Dial 死等：连不上会立刻返回错误，反复探测才能真正捕捉到
// 「进程刚起来、端口刚 listen」的那一刻。
//
// IPv4 / IPv6 双栈都要拨。Node 生态按环境不同可能只绑其中一边
// （比如 WXT dev server 只监听 [::1]，127.0.0.1 直接 connection refused），
// 只拨 IPv4 会把活的服务误判成启动失败。
func waitPort(ctx context.Context, port int) error {
	if port <= 0 {
		return fmt.Errorf("waitFor.port 配置无效: %d", port)
	}
	addrs := []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("[::1]:%d", port),
	}
	ticker := time.NewTicker(portPollInterval)
	defer ticker.Stop()

	for {
		for _, addr := range addrs {
			conn, err := net.DialTimeout("tcp", addr, portDialTimeout)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待端口 %d 超时", port)
		case <-ticker.C:
		}
	}
}

// logMatcher 在流式日志里找关键字。
//
// 关键字可能被切在两个数据块之间（比如 "Build com" + "plete"），所以每次
// 都保留 needle 长度减一的尾巴参与下一次匹配，否则会漏判。
type logMatcher struct {
	needle string
	tail   string
	hit    bool
}

func newLogMatcher(needle string) *logMatcher {
	return &logMatcher{needle: needle}
}

// feed 喂一段新日志，返回是否已命中。命中后永久为 true。
func (m *logMatcher) feed(chunk string) bool {
	if m.hit {
		return true
	}
	if m.needle == "" {
		return false
	}

	window := m.tail + chunk
	if strings.Contains(window, m.needle) {
		m.hit = true
		m.tail = ""
		return true
	}

	keep := len(m.needle) - 1
	if keep <= 0 {
		m.tail = ""
		return false
	}
	if len(window) > keep {
		m.tail = window[len(window)-keep:]
	} else {
		m.tail = window
	}
	return false
}

// resolveWaitTimeout 把配置里的秒数换算成等待上限。
func resolveWaitTimeout(wf *config.WaitFor) time.Duration {
	if wf != nil && wf.Timeout > 0 {
		return time.Duration(wf.Timeout) * time.Second
	}
	return defaultWaitTimeout
}
