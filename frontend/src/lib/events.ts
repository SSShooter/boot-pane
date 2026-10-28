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
