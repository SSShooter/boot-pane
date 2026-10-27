import type {
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
  return {
    cwd: "",
    origin: "repo",
    ...partial,
  } as Task;
}

const uniAppProject: ResolvedProject = {
  id: "my-shop-3f2a1b",
  name: "my-shop",
  path: "D:\\code\\my-shop",
  tags: ["uni-app", "vue3"],
  pinned: false,
  exists: true,
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
      origin: "local",
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
  hasRepoConfig: false,
  tasks: [
    task({
      id: "server",
      label: "启动 API",
      command: "go run ./cmd/api",
      waitFor: { type: "port", port: 8080 } as any,
    }),
  ],
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
  async listRunning() {
    return [];
  },
  async startTask(): Promise<void> {
    throw new Error("浏览器预览模式：进程管理不可用，请在 wails3 dev 下试");
  },
  async stopTask(): Promise<void> {},
};
