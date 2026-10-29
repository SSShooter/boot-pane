/**
 * 后端事件契约。
 *
 * 事件名必须和 Go 侧 internal/process/manager.go 里的 EventXxx 常量逐字一致，
 * 改一边就要改另一边——Wails 不做任何名字校验，写错了只会静默收不到事件。
 */
export const EVENT_RUNTIME = "bootpane:runtime";
export const EVENT_LOG = "bootpane:log";
export const EVENT_STATS = "bootpane:stats";
export const EVENT_CLOSE_REQUEST = "bootpane:close-request";

/**
 * 日志面板状态变了（开标签、切标签、关标签、删项目）。
 *
 * 面板只存在于一个窗口里：内嵌在主窗口，或者弹成独立窗口。状态由「正在操作
 * 的那个窗口」发出，另一个窗口原样采纳——两边各存一份标签只会在切来切去之后
 * 对不上。
 */
export const EVENT_LOG_STATE = "bootpane:log-state";

/** 清空某个标签的日志内容。另一个窗口也攒着同一份输出，得一起清。 */
export const EVENT_LOG_CLEAR = "bootpane:log-clear";

/** 独立窗口请求把面板收回主窗口（用户点了独立窗口上的「内嵌」）。 */
export const EVENT_LOG_EMBED = "bootpane:log-embed";

/**
 * 刚启动的窗口向另一个窗口要一份当前的面板状态。
 *
 * 独立窗口拿 URL 里的快照是最新的一份，但两者之间的窗口里主窗口可能又
 * 开了新标签。相比在弹窗那一瞬间把状态再推一遍（那时新窗口还没订阅上），
 * 让新窗口主动问一句才没有时间差。
 */
export const EVENT_LOG_STATE_ASK = "bootpane:log-state-ask";

/**
 * 日志独立窗口关掉了。
 *
 * 名字必须和 Go 侧 log_window.go 里的 EventLogWindowClosed 逐字一致。
 * 不区分「点内嵌关的」和「直接叉掉的」：两种情况下主窗口都要把面板接回来。
 */
export const EVENT_LOG_WINDOW_CLOSED = "bootpane:log-window-closed";

/**
 * 独立窗口入口地址里承载面板状态的参数名。
 *
 * 必须和 Go 侧 log_window.go 里的 logWindowQuery 逐字一致。
 */
export const LOG_WINDOW_QUERY = "log";

/** 日志增量。后端已按 50ms / 32KB 节流，前端直接追加即可。 */
export interface LogPayload {
  projectId: string;
  taskId: string;
  chunk: string;
}

/** 资源采样。1s 一次，和 runtime 快照分开推，避免整张卡片重渲染。 */
export interface StatsPayload {
  projectId: string;
  taskId: string;
  cpuPercent: number;
  memoryBytes: number;
  processes: number;
}

/**
 * 关闭窗口时还有任务在跑，后端拦下了这次关闭。
 *
 * 只给 id：任务的中文名前端本来就有，后端不用为了拼一句话去反查配置。
 */
export interface CloseRequestPayload {
  tasks: { projectId: string; taskId: string }[];
}

/** 任务在状态与日志上的唯一键。 */
export function taskKey(projectId: string, taskId: string): string {
  return `${projectId}/${taskId}`;
}

/**
 * 日志面板状态快照。
 *
 * 只带定位信息，不带日志正文：正文两个窗口各自从事件流里攒，缺历史时
 * 从后端环形缓冲补一次。source 是本窗口的随机标识，用来忽略自己发出的回音。
 */
export interface LogStatePayload {
  source: string;
  tabs: { key: string; projectId: string; taskId: string }[];
  key: string;
}

/** 清空某个标签的载荷。 */
export interface LogClearPayload {
  source: string;
  key: string;
}
