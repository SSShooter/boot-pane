import { ConfigService, LogWindowService, ProcessService, AppService } from "../../bindings/bootpane";
import type {
  ImportResult,
  ResolvedProject,
  Settings,
} from "../../bindings/bootpane/internal/config/models";
import type { Runtime } from "../../bindings/bootpane/internal/process/models";
import type { Editor } from "../../bindings/bootpane/internal/editors/models";
import type { Listener } from "../../bindings/bootpane/internal/portinfo/models";
import type { RestartResult } from "../../bindings/bootpane/models";
import { mockApi } from "./devmock";

/**
 * WebView2 会注入 chrome.webview，普通浏览器没有。
 * 用它判断当前是不是跑在桌面壳里，比探测 Wails 内部字段稳定。
 */
const inWebView = typeof window !== "undefined" && !!(window as any).chrome?.webview;

/** 纯浏览器里跑 vite dev 时走假数据，方便调 UI。 */
export const isPreview = import.meta.env.DEV && !inWebView;

export interface Api {
  listProjects(): Promise<ResolvedProject[]>;
  addProject(path: string): Promise<ResolvedProject>;
  removeProject(id: string): Promise<void>;
  openProjectDir(id: string): Promise<void>;
  openConfigDir(): Promise<void>;
  /** 用系统浏览器打开任务对应的页面（openUrl 或按端口兜底）。 */
  openTaskPage(projectId: string, taskId: string): Promise<void>;
  setUseRepoConfig(id: string, enabled: boolean): Promise<void>;
  /** 设置任务星标。只写本机注册表，不碰仓库配置。 */
  setStar(projectId: string, taskId: string, starred: boolean): Promise<void>;
  buildConfigPrompt(id: string): Promise<string>;
  importTasks(id: string, raw: string): Promise<ImportResult>;
  /** 拉当前生效配置的完整 JSON 文本，供编辑框直接改。 */
  configJson(id: string): Promise<string>;
  /** 保存编辑过的配置。写入目标跟着项目的配置来源走。 */
  saveConfigJson(id: string, raw: string): Promise<ImportResult>;
  /** 读本机偏好（自动打开日志这类开关）。偏好存在 workspace.json 顶层。 */
  getSettings(): Promise<Settings>;
  /** 切换「启动任务后自动打开日志」。 */
  setAutoOpenLogOnStart(enabled: boolean): Promise<void>;
  /** 设置「打开目录」用的命令，空串表示系统文件管理器。 */
  setOpenWith(command: string): Promise<void>;
  /** 扫描本机装了哪些编辑器，供设置页直接选。 */
  detectEditors(): Promise<Editor[]>;
  listRunning(): Promise<Runtime[]>;
  startTask(projectId: string, taskId: string): Promise<void>;
  stopTask(projectId: string, taskId: string): Promise<void>;
  /** 拉某个任务已缓冲的日志全量，用于首次打开日志面板。 */
  getTaskLog(projectId: string, taskId: string): Promise<string>;
  /** 批量查询端口占用。和任务状态分开——端口活着不代表是 bootpane 起的。 */
  checkPorts(ports: number[]): Promise<Listener[]>;
  /** 重启任务：端口被外部进程占着时先杀掉整棵进程树再启动。 */
  restartTask(projectId: string, taskId: string): Promise<RestartResult>;
  /** 告诉后端：关闭提示已经弹出来了，别走超时兜底。 */
  acknowledgeClose(): Promise<void>;
  /** 用户确认停止所有任务并退出。 */
  confirmQuit(): Promise<void>;
  /** 用户取消了这次关闭，窗口留着。 */
  cancelQuit(): Promise<void>;
  /** 把日志面板弹成独立窗口，state 是面板状态的序列化快照。 */
  detachLogWindow(state: string): Promise<void>;
  /** 把已经弹出的日志窗口拉到前面；返回窗口是否还在。 */
  focusLogWindow(): Promise<boolean>;
}

/**
 * 真实后端适配层。
 *
 * Wails 生成的切片返回值都是 `T[] | null`（对应 Go 的 nil slice），而界面里
 * 几乎每处都要遍历这些列表。统一在这里折成空数组，调用方就不用每处判空，
 * 也不会因为后端返回 null 在渲染时炸掉。
 */
const realApi: Api = {
  listProjects: async () => (await ConfigService.ListProjects()) ?? [],
  addProject: (path) => ConfigService.AddProject(path),
  removeProject: (id) => ConfigService.RemoveProject(id),
  openProjectDir: (id) => ConfigService.OpenProjectDir(id),
  openConfigDir: () => ConfigService.OpenConfigDir(),
  openTaskPage: (projectId, taskId) => ConfigService.OpenTaskPage(projectId, taskId),
  setUseRepoConfig: (id, enabled) => ConfigService.SetUseRepoConfig(id, enabled),
  setStar: (projectId, taskId, starred) => ConfigService.SetStar(projectId, taskId, starred),
  buildConfigPrompt: (id) => ConfigService.BuildConfigPrompt(id),
  importTasks: (id, raw) => ConfigService.ImportTasks(id, raw),
  configJson: (id) => ConfigService.ConfigJSON(id),
  saveConfigJson: (id, raw) => ConfigService.SaveConfigJSON(id, raw),
  getSettings: () => ConfigService.GetSettings(),
  setAutoOpenLogOnStart: (enabled) => ConfigService.SetAutoOpenLogOnStart(enabled),
  setOpenWith: (command) => ConfigService.SetOpenWith(command),
  detectEditors: async () => (await ConfigService.DetectEditors()) ?? [],
  listRunning: async () => (await ProcessService.ListRunning()) ?? [],
  startTask: (projectId, taskId) => ProcessService.StartTask(projectId, taskId),
  stopTask: (projectId, taskId) => ProcessService.StopTask(projectId, taskId),
  getTaskLog: (projectId, taskId) => ProcessService.TaskLog(projectId, taskId),
  checkPorts: async (ports) => (await ProcessService.CheckPorts(ports)) ?? [],
  restartTask: (projectId, taskId) => ProcessService.RestartTask(projectId, taskId),
  acknowledgeClose: () => AppService.AcknowledgeClose(),
  confirmQuit: () => AppService.ConfirmQuit(),
  cancelQuit: () => AppService.CancelQuit(),
  detachLogWindow: (state) => LogWindowService.Open(state),
  focusLogWindow: () => LogWindowService.Focus(),
};

export const api: Api = isPreview ? (mockApi as unknown as Api) : realApi;
