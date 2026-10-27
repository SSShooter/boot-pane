package main

import (
	"embed"
	"log"
	"os"

	"bootpane/internal/config"
	"bootpane/internal/process"

	"github.com/wailsapp/wails/v3/pkg/application"
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

	procMgr := process.NewManager()

	app := application.New(application.Options{
		Name:        "bootpane",
		Description: "项目启动器",
		Services: []application.Service{
			application.NewService(NewConfigService(store)),
			application.NewService(NewProcessService(procMgr, store)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
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

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
