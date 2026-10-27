import type {
  ImportResult,
  ResolvedProject,
  Task,
} from "../../bindings/bootpane/internal/config/models";

/**
 * 在纯浏览器里（vite dev，没有 Wails 运行时）使用的假数据。
 *
 * 用途只有一个：调 UI 时不用每次起整个桌面应用。生产构建里这段代码会被
 * import.meta.env.DEV 判断摇掉，不会进入产物。
 */

function task(partial: Partial<Task> & { id: string; label: string; command: string }): Task {
  return { cwd: "", ...partial } as Task;
}

const uniAppProject: ResolvedProject = {
  id: "my-shop-3f2a1b",
  name: "my-shop",
  path: "D:\\code\\my-shop",
  tags: ["uni-app", "vue3"],
  pinned: false,
  exists: true,
  useRepoConfig: false,
  configSource: "global",
  hasRepoConfig: true,
  tasks: [
    task({
      id: "web",
      label: "启动网页端",
      group: "前端",
      command: "pnpm dev:h5",
      waitFor: { type: "port", port: 5173 } as any,
    }),
    task({
      id: "mp-weixin",
      label: "启动小程序端",
      group: "小程序",
      command: "pnpm dev:mp-weixin",
      dependsOn: ["web"],
      waitFor: { type: "log", log: "Build complete" } as any,
    }),
    task({
      id: "mock",
      label: "Mock 服务",
      group: "服务",
      command: "node ./mock/server.js",
      cwd: "tools",
      waitFor: { type: "port", port: 3001 } as any,
    }),
  ],
  pipelines: [{ id: "all", label: "全启", tasks: ["web", "mp-weixin"], mode: "serial" }],
  issues: [],
} as ResolvedProject;

const backendProject: ResolvedProject = {
  id: "backend-api-88c04d",
  name: "backend-api",
  path: "D:\\code\\backend-api",
  tags: ["go"],
  pinned: false,
  exists: true,
  useRepoConfig: false,
  configSource: "global",
  hasRepoConfig: false,
  tasks: [],
  pipelines: [],
  issues: [],
} as ResolvedProject;

const brokenProject: ResolvedProject = {
  id: "legacy-admin-7ab9e2",
  name: "legacy-admin",
  path: "E:\\old\\legacy-admin",
  tags: [],
  pinned: false,
  exists: false,
  useRepoConfig: false,
  configSource: "global",
  hasRepoConfig: false,
  tasks: [],
  pipelines: [],
  issues: [{ level: "error", message: "项目目录不存在或不可访问" }],
} as ResolvedProject;

export const mockProjects: ResolvedProject[] = [
  uniAppProject,
  backendProject,
  brokenProject,
];

const mockPrompt = `我要给一个项目配置 bootpane 的启动按钮。

【使用方式】请把这段内容放到项目根目录下使用：
    D:\\code\\my-shop
在该目录里打开你的 AI 工具再贴这段话。

只输出一个 json 代码块，不要任何解释文字。

（浏览器预览模式下这里是截断的示例 prompt，真实内容由 Go 侧生成）`;

export const mockApi = {
  async listProjects(): Promise<ResolvedProject[]> {
    return mockProjects;
  },
  async addProject(path: string): Promise<ResolvedProject> {
    return { ...backendProject, id: "mock-" + path, name: path } as ResolvedProject;
  },
  async removeProject(): Promise<void> {},
  async openProjectDir(): Promise<void> {},
  async openConfigDir(): Promise<void> {},
  async setUseRepoConfig(): Promise<void> {
    throw new Error("浏览器预览模式：无法切换配置来源");
  },
  async buildConfigPrompt(): Promise<string> {
    return mockPrompt;
  },
  async importTasks(_id: string, raw: string): Promise<ImportResult> {
    if (!raw.trim()) {
      throw new Error("没找到 JSON 内容，请确认粘贴的是完整的配置");
    }
    throw new Error("浏览器预览模式：导入需要在桌面应用里操作");
  },
  async listRunning() {
    return [];
  },
  async startTask(): Promise<void> {
    throw new Error("浏览器预览模式：进程管理不可用，请在 wails3 dev 下试");
  },
  async stopTask(): Promise<void> {},
};
