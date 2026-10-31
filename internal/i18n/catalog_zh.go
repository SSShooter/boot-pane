package i18n

// zhMessages 是中文文案。key 与 enMessages 一一对应，缺条目会有单测兜底。
var zhMessages = map[string]string{
	"app.description": "项目启动器",

	// 注册表读写
	"store.projectNotFound": "项目不存在",
	"store.configDir":       "无法定位用户配置目录: %v",
	"store.mkdir":           "创建配置目录失败: %v",
	"store.readFile":        "读取 %s 失败: %v",
	"store.parseWorkspace":  "%s 格式错误: %v",
	"store.badPath":         "路径无效: %v",
	"store.dirMissing":      "目录不存在: %s",
	"store.emptyIds":        "项目和任务 id 不能为空",
	"store.dirUnreadable":   "项目目录不存在或不可访问",
	"store.repoParse":       "%s 解析失败: %v",
	"store.repoMissing":     "已切换为项目内配置，但仓库里没有 %s",
	"store.repoWrite":       "写入 %s 失败: %v",
	"store.langUnsupported": "不支持的语言: %q",

	// 配置校验
	"validate.idMissing":          "任务 %q 缺少 id",
	"validate.idDuplicate":        "任务 id 重复",
	"validate.commandMissing":     "未配置启动命令",
	"validate.cwdMissing":         "工作目录不存在: %s",
	"validate.badOpenURL":         "openUrl 不是合法的 http(s) 地址: %s",
	"validate.depMissing":         "依赖的任务 %q 不存在",
	"validate.portInvalid":        "waitFor.port 不是合法端口",
	"validate.portDuplicate":      "端口 %d 与任务 %q 重复",
	"validate.logKeywordMissing":  "waitFor.type 为 log 但未填写关键字",
	"validate.waitTypeUnknown":    "未知的 waitFor.type: %q",
	"validate.pipelineRefMissing": "编排 %q 引用了不存在的任务 %q",
	"validate.depCycle":           "依赖成环: %s",

	// 导入与解析
	"import.noTasks": "配置里没有任何 task",
	"import.noJSON":  "没找到 JSON 内容，请确认粘贴的是完整的配置",
	"import.badJSON": "JSON 格式有误：%v",

	// 进程启动与就绪判定
	"run.alreadyRunning":    "「%s」已经在运行中",
	"run.cwdMissing":        "工作目录不存在：%s",
	"run.treeFail":          "创建进程树失败：%v",
	"run.spawnFail":         "启动失败：%v",
	"run.exitedImmediately": "进程启动后立即退出（退出码 %d）",
	"run.waitLogTimeout":    "等待日志 %q 超时",
	"run.waitTypeUnknown":   "未知的 waitFor 类型：%s",
	"run.notImplemented":    "进程管理尚未接入",
	"process.listener":      "%s（PID %d）",
	"process.pidOnly":       "PID %d",

	// 打开方式
	"host.openWithEmpty":   "打开方式不能为空",
	"host.commandNotFound": "找不到命令 %q，请确认它已安装并在 PATH 里",

	// 服务层
	"svc.taskNoPage":     "任务 %q 没有可打开的页面",
	"svc.taskNotFound":   "任务 %q 不存在",
	"svc.importRepoMode": "该项目当前使用项目内配置，请先切回全局再导入",
	"svc.importFailed":   "导入的配置有错误，已取消写入",
	"svc.saveFailed":     "配置有错误，未保存",

	// 重启外部实例
	"restart.noPort":      "该任务没有配置端口，无法判断外部实例",
	"restart.killFailed":  "终止 %s 失败：%v",
	"restart.portStuck":   "已终止 %d 个进程，但端口 %d 仍被占用",
	"restart.started":     "已启动",
	"restart.done":        "已终止外部实例并重新启动",
	"restart.portTimeout": "端口 %d 在 %v 内没有释放",

	// 日志独立窗口
	"logWindow.title":       "运行日志 · BootPane",
	"logWindow.stateEmpty":  "日志面板状态为空",
	"logWindow.appNotReady": "应用尚未初始化",
}
