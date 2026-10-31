package i18n

// enMessages 是英文文案，同时充当缺省兜底：其他语言缺条目时回落到这里。
var enMessages = map[string]string{
	"app.description": "Project launcher",

	// 注册表读写
	"store.projectNotFound": "project not found",
	"store.configDir":       "cannot locate the user config directory: %v",
	"store.mkdir":           "failed to create the config directory: %v",
	"store.readFile":        "failed to read %s: %v",
	"store.parseWorkspace":  "%s is malformed: %v",
	"store.badPath":         "invalid path: %v",
	"store.dirMissing":      "folder does not exist: %s",
	"store.emptyIds":        "project and task ids must not be empty",
	"store.dirUnreadable":   "project folder is missing or inaccessible",
	"store.repoParse":       "failed to parse %s: %v",
	"store.repoMissing":     "switched to repository config, but the repository has no %s",
	"store.repoWrite":       "failed to write %s: %v",
	"store.langUnsupported": "unsupported language: %q",

	// 配置校验
	"validate.idMissing":          "task %q has no id",
	"validate.idDuplicate":        "duplicate task id",
	"validate.commandMissing":     "no launch command configured",
	"validate.cwdMissing":         "working folder does not exist: %s",
	"validate.badOpenURL":         "openUrl is not a valid http(s) URL: %s",
	"validate.depMissing":         "required task %q does not exist",
	"validate.portInvalid":        "waitFor.port is not a valid port",
	"validate.portDuplicate":      "port %d is already used by task %q",
	"validate.logKeywordMissing":  "waitFor.type is log but no keyword is set",
	"validate.waitTypeUnknown":    "unknown waitFor.type: %q",
	"validate.pipelineRefMissing": "pipeline %q references missing task %q",
	"validate.depCycle":           "dependency cycle: %s",

	// 导入与解析
	"import.noTasks": "the config contains no tasks",
	"import.noJSON":  "no JSON found, make sure the whole config was pasted",
	"import.badJSON": "invalid JSON: %v",

	// 进程启动与就绪判定
	"run.alreadyRunning":    "“%s” is already running",
	"run.cwdMissing":        "working folder does not exist: %s",
	"run.treeFail":          "failed to create the process tree: %v",
	"run.spawnFail":         "failed to start: %v",
	"run.exitedImmediately": "the process exited right after starting (exit code %d)",
	"run.waitLogTimeout":    "timed out waiting for log %q",
	"run.waitTypeUnknown":   "unknown waitFor type: %s",
	"run.notImplemented":    "process management is not implemented for this platform",
	"process.listener":      "%s (PID %d)",
	"process.pidOnly":       "PID %d",

	// 打开方式
	"host.openWithEmpty":   "the open-with command must not be empty",
	"host.commandNotFound": "command %q not found, make sure it is installed and on PATH",

	// 服务层
	"svc.taskNoPage":     "task %q has no page to open",
	"svc.taskNotFound":   "task %q does not exist",
	"svc.importRepoMode": "this project uses repository config, switch back to global before importing",
	"svc.importFailed":   "the imported config has errors, nothing was written",
	"svc.saveFailed":     "the config has errors and was not saved",

	// 重启外部实例
	"restart.noPort":      "this task has no port configured, so external instances cannot be detected",
	"restart.killFailed":  "failed to terminate %s: %v",
	"restart.portStuck":   "terminated %d processes, but port %d is still in use",
	"restart.started":     "started",
	"restart.done":        "terminated the external instance and restarted",
	"restart.portTimeout": "port %d was not released within %v",

	// 日志独立窗口
	"logWindow.title":       "Run logs · BootPane",
	"logWindow.stateEmpty":  "the log panel state is empty",
	"logWindow.appNotReady": "the app is not initialized yet",
}
