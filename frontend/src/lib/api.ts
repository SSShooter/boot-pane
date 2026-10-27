import { ConfigService, ProcessService } from "../../bindings/bootpane";
import type {
  ImportResult,
  ResolvedProject,
} from "../../bindings/bootpane/internal/config/models";
import type { Runtime } from "../../bindings/bootpane/internal/process/models";
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
  buildConfigPrompt(id: string): Promise<string>;
  importTasks(id: string, raw: string): Promise<ImportResult>;
  listRunning(): Promise<Runtime[]>;
  startTask(projectId: string, taskId: string): Promise<void>;
  stopTask(projectId: string, taskId: string): Promise<void>;
}

const realApi: Api = {
  listProjects: () => ConfigService.ListProjects(),
  addProject: (path) => ConfigService.AddProject(path),
  removeProject: (id) => ConfigService.RemoveProject(id),
  openProjectDir: (id) => ConfigService.OpenProjectDir(id),
  openConfigDir: () => ConfigService.OpenConfigDir(),
  setUseRepoConfig: (id, enabled) => ConfigService.SetUseRepoConfig(id, enabled),
  buildConfigPrompt: (id) => ConfigService.BuildConfigPrompt(id),
  importTasks: (id, raw) => ConfigService.ImportTasks(id, raw),
  listRunning: () => ProcessService.ListRunning(),
  startTask: (projectId, taskId) => ProcessService.StartTask(projectId, taskId),
  stopTask: (projectId, taskId) => ProcessService.StopTask(projectId, taskId),
};

export const api: Api = isPreview ? (mockApi as unknown as Api) : realApi;
