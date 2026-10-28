package main

import (
	"embed"
	"log"
	"os"
	"sync/atomic"

	"bootpane/internal/config"
	"bootpane/internal/process"

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

	// 首次运行时把当前工作目录登记为初始项目，避免打开应用看到一片空白。
	// 打包后工作目录是程序所在目录，通常没有 go.mod，这段不会生效。
	if wd, err := os.Getwd(); err == nil {
		if _, statErr := os.Stat(wd + string(os.PathSeparator) + "go.mod"); statErr == nil {
			_ = store.SeedIfEmpty(wd)
		}
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
	guard := newQuitGuard(procMgr.Running, func() {
		if a := appPtr.Load(); a != nil {
			a.Quit()
		}
	}, emit)

	app := application.New(application.Options{
		Name:        "bootpane",
		Description: "项目启动器",
		Services: []application.Service{
			application.NewService(NewConfigService(store)),
			application.NewService(NewProcessService(procMgr, store)),
			application.NewService(NewAppService(guard)),
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

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "bootpane",
		Width:            1180,
		Height:           760,
		BackgroundColour: application.NewRGB(250, 250, 249),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	// 拦在窗口真正关掉之前。取消掉这次关闭，让前端弹确认框，
	// 用户确认后再走 app.Quit，OnShutdown 才有机会清理进程树。
	window.RegisterHook(events.Common.WindowClosing, guard.HandleWindowClosing)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
