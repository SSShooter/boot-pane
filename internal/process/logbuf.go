package process

import (
	"bytes"
	"sync"
)

// defaultLogBytes 是单个任务保留的日志上限。
// 256KB 大概能装下 vite 一次完整冷启动的输出，再多对排查问题没有增量价值，
// 而且会让「打开日志面板」这个动作变慢。
const defaultLogBytes = 256 * 1024

// logRing 是单个任务的日志环形缓冲。
//
// 只留最近的一段：前端打开面板时一次性拉全量，之后靠事件增量追加。
// 裁剪时尽量对齐到换行，否则从中间切断会把一行 UTF-8 字符切成乱码。
type logRing struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newLogRing(max int) *logRing {
	if max <= 0 {
		max = defaultLogBytes
	}
	return &logRing{max: max}
}

func (r *logRing) append(s string) {
	if s == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf = append(r.buf, s...)
	if len(r.buf) <= r.max {
		return
	}

	cut := len(r.buf) - r.max
	if idx := bytes.IndexByte(r.buf[cut:], '\n'); idx >= 0 {
		cut += idx + 1
	}
	// 用 make+copy 而不是 append(r.buf[:0], ...)：后者会让底层数组
	// 一直保持在历史峰值大小，长时间运行后内存降不下来。
	trimmed := make([]byte, len(r.buf)-cut)
	copy(trimmed, r.buf[cut:])
	r.buf = trimmed
}

func (r *logRing) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}
