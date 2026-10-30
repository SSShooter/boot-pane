package main

import (
	"bootpane/internal/quitguard"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AppService 把「关闭应用」这一侧的交互暴露给前端。
//
// 单独一个服务而不是塞进 ProcessService：它管的是应用生命周期，
// 和启动/停止任务不是一回事。拦不拦、拦下来之后怎么等回执这些规则都在
// internal/quitguard 里，这里只把窗口事件和前端调用接到那个状态机上。
type AppService struct {
	guard *quitguard.Guard
}

// NewAppService 构造应用生命周期服务。
func NewAppService(g *quitguard.Guard) *AppService {
	return &AppService{guard: g}
}

// AcknowledgeClose 表示界面已经收到关闭提示并弹出了确认框。
func (s *AppService) AcknowledgeClose() {
	s.guard.AcknowledgeClose()
}

// ConfirmQuit 用户确认停止所有任务并退出。
func (s *AppService) ConfirmQuit() {
	s.guard.ConfirmQuit()
}

// CancelQuit 用户取消了这次关闭。
func (s *AppService) CancelQuit() {
	s.guard.CancelQuit()
}

// windowClosing 是 WindowClosing 钩子的入口。
//
// 故意不导出：钩子签名里带 *application.WindowEvent，导出就会被绑到前端，
// 让生成器把整个 Wails 窗口类型连同它的依赖翻译成 TS 没有意义。
// main 里注册的是方法值，不需要导出。
func (s *AppService) windowClosing(e *application.WindowEvent) {
	s.guard.HandleClosing(e)
}
