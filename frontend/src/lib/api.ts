import { ConfigService, ProcessService } from "../../bindings/bootpane";
import type {
  ImportResult,
  ResolvedProject,
} from "../../bindings/bootpane/internal/config/models";
import type { Runtime } from "../../bindings/bootpane/internal/process/models";
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
  setUseRepoConfig(id: string, enabled: boolean): Promise<void>;
  /** 设置任务星标。只写本机注册表，不碰仓库配置。 */
  setStar(projectId: string, taskId: string, starred: boolean): Promise<void>;
  buildConfigPrompt(id: string): Promise<string>;
  importTasks(id: string, raw: string): Promise<ImportResult>;
  /** 拉当前生效配置的完整 JSON 文本，供编辑框直接改。 */
  configJson(id: string): Promise<string>;
  /** 保存编辑过的配置。写入目标跟着项目的配置来源走。 */
  saveConfigJson(id: string, raw: string): Promise<ImportResult>;
  listRunning(): Promise<Runtime[]>;
  startTask(projectId: string, taskId: string): Promise<void>;
  stopTask(projectId: string, taskId: string): Promise<void>;
  /** 拉某个任务已缓冲的日志全量，用于首次打开日志面板。 */
  getTaskLog(projectId: string, taskId: string): Promise<string>;
  /** 批量查询端口占用。和任务状态分开——端口活着不代表是 bootpane 起的。 */
  checkPorts(ports: number[]): Promise<Listener[]>;
  /** 重启任务：端口被外部进程占着时先杀掉整棵进程树再启动。 */
  restartTask(projectId: string, taskId: string): Promise<RestartResult>;
}

const realApi: Api = {
  listProjects: () => ConfigService.ListProjects(),
  addProject: (path) => ConfigService.AddProject(path),
  removeProject: (id) => ConfigService.RemoveProject(id),
  openProjectDir: (id) => ConfigService.OpenProjectDir(id),
  openConfigDir: () => ConfigService.OpenConfigDir(),
  setUseRepoConfig: (id, enabled) => ConfigService.SetUseRepoConfig(id, enabled),
  setStar: (projectId, taskId, starred) => ConfigService.SetStar(projectId, taskId, starred),
  buildConfigPrompt: (id) => ConfigService.BuildConfigPrompt(id),
  importTasks: (id, raw) => ConfigService.ImportTasks(id, raw),
  configJson: (id) => ConfigService.ConfigJSON(id),
  saveConfigJson: (id, raw) => ConfigService.SaveConfigJSON(id, raw),
  listRunning: () => ProcessService.ListRunning(),
  startTask: (projectId, taskId) => ProcessService.StartTask(projectId, taskId),
  stopTask: (projectId, taskId) => ProcessService.StopTask(projectId, taskId),
  getTaskLog: (projectId, taskId) => ProcessService.TaskLog(projectId, taskId),
  checkPorts: (ports) => ProcessService.CheckPorts(ports),
  restartTask: (projectId, taskId) => ProcessService.RestartTask(projectId, taskId),
};

export const api: Api = isPreview ? (mockApi as unknown as Api) : realApi;
