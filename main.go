package main

import (
	"embed"
	"log"
	"runtime"
	"sync/atomic"

	"bootpane/internal/config"
	"bootpane/internal/process"
	"bootpane/internal/quitguard"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	store, err := config.NewStore()
	if err != nil {
		log.Fatalf("初始化配置失败: %v", err)
	}

	// 事件出口用原子指针延迟绑定：进程管理器要先进 Services，而 app 得等
	// application.New 才存在，中间这段时间采样协程可能已经跑起来了。
	// 直接读 application.Get() 会和 New 里的赋值构成数据竞争，这里显式同步。
	var appPtr atomic.Pointer[application.App]

	emit := func(event string, data any) {
		if a := appPtr.Load(); a != nil {
			a.Event.Emit(event, data)
		}
	}

	procMgr := process.NewManager(emit)

	// 关窗口时如果有任务在跑，先让前端问一句：直接退会把 dev server 全掐了，
	// 用户可能只是手滑点了叉。
	guard := quitguard.New(procMgr.Running, func() {
		if a := appPtr.Load(); a != nil {
			a.Quit()
		}
	}, emit)
	appService := NewAppService(guard)

	// 日志独立窗口的服务要在 New 之前注册，但它创建窗口要用的 app 只能 New 之后
	// 才有，所以先构造、再 Attach。
	logWindow := NewLogWindowService()

	app := application.New(application.Options{
		Name:        "BootPane",
		Description: "项目启动器",
		Services: []application.Service{
			application.NewService(NewConfigService(store)),
			application.NewService(NewProcessService(procMgr, store)),
			application.NewService(appService),
			application.NewService(logWindow),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// 退出前把整棵进程树清掉，不然关掉窗口后 node 还占着端口。
		OnShutdown: procMgr.Shutdown,
	})
	appPtr.Store(app)
	logWindow.attach(app)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "BootPane",
		Width:            1180,
		Height:           760,
		// 和 style.css 的 --bg 保持一致：首帧渲染前窗口是这块色，对不上就会闪一下白。
		BackgroundColour: application.NewRGB(13, 13, 15),
		URL:              "/",
		// Windows 上去掉系统标题栏，改由前端 TitleBar.svelte 画：系统标题栏
		// 里已经写着一次应用名，再和侧栏品牌并列就重复了，还白占一行高度。
		// macOS 相反，保留系统交通灯、只藏标题栏文字，所以这里不能一刀切。
		// 前端 lib/chrome.ts 的 customChrome 必须和这个判断保持一致。
		Frameless: runtime.GOOS == "windows",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	// 拦在窗口真正关掉之前。取消掉这次关闭，让前端弹确认框，
	// 用户确认后再走 app.Quit，OnShutdown 才有机会清理进程树。
	window.RegisterHook(events.Common.WindowClosing, appService.windowClosing)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
