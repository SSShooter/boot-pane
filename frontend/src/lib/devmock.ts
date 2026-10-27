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
  // 默认值放后面展开，否则 cwd 会插到 id 前面，预览里的 JSON 字段顺序很怪。
  return { ...partial, cwd: partial.cwd ?? "" } as Task;
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
  starredTaskIds: ["web", "mock"],
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
  tasks: [
    task({
      id: "serve",
      label: "启动 API",
      group: "服务",
      command: "go run ./cmd/api",
      waitFor: { type: "port", port: 8080 } as any,
    }),
  ],
  pipelines: [],
  issues: [],
  // 故意跨项目预置一个星标，用来预览聚合视图里的项目徽章。
  starredTaskIds: ["serve"],
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
  starredTaskIds: [],
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

/**
 * 浏览器预览模式下的假运行状态。
 *
 * 只为了让 `npm run dev` 能看清运行中卡片的样式（CPU/内存徽章、日志面板），
 * 不模拟真实的事件流——真要验行为还是得跑 wails3 dev。
 */
const mockRuntimes = new Map<string, any>();

// 预置一个运行中的任务，这样 `npm run dev` 一打开就能看到资源徽章和日志面板，
// 不用先点启动（预览模式下没有事件流，点了也不会真的跑起来）。
mockRuntimes.set("my-shop-3f2a1b/web", {
  projectId: "my-shop-3f2a1b",
  taskId: "web",
  state: "running",
  pid: 24680,
  ready: true,
  startedAt: new Date().toISOString(),
  exitCode: 0,
  cpuPercent: 7.3,
  memoryBytes: 214 * 1024 * 1024,
  processes: 4,
});

const mockLogSample = `\u001b[2mVITE v8.2.4\u001b[22m  ready in 412 ms

  \u001b[32m➜\u001b[39m  \u001b[1mLocal:\u001b[22m   \u001b[36mhttp://localhost:\u001b[1m5173\u001b[22m/\u001b[39m
  \u001b[32m➜\u001b[39m  \u001b[1mNetwork:\u001b[22m \u001b[2muse --host to expose\u001b[39m\u001b[22m
  \u001b[32m➜\u001b[39m  \u001b[1mpress \u001b[22m\u001b[2mh + enter\u001b[22m to show help

\u001b[2m12:04:31\u001b[22m \u001b[36m[vite]\u001b[39m \u001b[32mhmr update\u001b[39m \u001b[2m/src/pages/index.vue\u001b[22m
\u001b[2m12:04:38\u001b[22m \u001b[36m[vite]\u001b[39m \u001b[32mhmr update\u001b[39m \u001b[2m/src/components/TaskCard.vue\u001b[22m
`;

/** 预览模式下被「外部进程」占着的端口，重启后会从这里移除。 */
const mockOccupied: Record<number, { pid: number; process: string }> = {
  3001: { pid: 47772, process: "node.exe" },
};

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
  /** 假星标：直接改内存里的 mock 数据，刷新页面就回到预置状态。 */
  async setStar(projectId: string, taskId: string, starred: boolean): Promise<void> {
    const p = mockProjects.find((x) => x.id === projectId);
    if (!p) return;
    const ids = p.starredTaskIds.filter((id) => id !== taskId);
    p.starredTaskIds = starred ? [...ids, taskId] : ids;
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
  /**
   * 预览模式下的配置编辑：直接改内存里的 mock 项目。
   *
   * 不落盘、不做真实校验（工作目录是否存在这类判断在这里没有意义），
   * 只保证「改完能看到卡片变化」这条链路是通的，方便调弹窗的交互和样式。
   */
  async configJson(id: string): Promise<string> {
    const p = mockProjects.find((x) => x.id === id);
    if (!p) throw new Error("项目不存在");
    return (
      JSON.stringify(
        {
          version: 1,
          project: { name: p.name, tags: p.tags },
          tasks: p.tasks,
          pipelines: p.pipelines,
        },
        null,
        2,
      ) + "\n"
    );
  },
  async saveConfigJson(id: string, raw: string): Promise<ImportResult> {
    const p = mockProjects.find((x) => x.id === id);
    if (!p) throw new Error("项目不存在");

    let parsed: any;
    try {
      parsed = JSON.parse(raw);
    } catch (e) {
      return { ok: false, imported: 0, error: "JSON 格式有误：" + String(e), issues: [] };
    }
    if (!Array.isArray(parsed?.tasks)) {
      return { ok: false, imported: 0, error: "tasks 必须是数组", issues: [] };
    }

    p.tasks = parsed.tasks;
    if (Array.isArray(parsed?.pipelines)) p.pipelines = parsed.pipelines;
    if (parsed?.project?.name) p.name = parsed.project.name;
    if (Array.isArray(parsed?.project?.tags)) p.tags = parsed.project.tags;
    return { ok: true, imported: p.tasks.length, issues: [] };
  },
  async listRunning() {
    return [...mockRuntimes.values()];
  },
  async startTask(projectId: string, taskId: string): Promise<void> {
    mockRuntimes.set(`${projectId}/${taskId}`, {
      projectId,
      taskId,
      state: "running",
      pid: 12345,
      ready: true,
      startedAt: new Date().toISOString(),
      exitCode: 0,
      cpuPercent: 12.4,
      memoryBytes: 148 * 1024 * 1024,
      processes: 3,
    });
  },
  async stopTask(projectId: string, taskId: string): Promise<void> {
    mockRuntimes.delete(`${projectId}/${taskId}`);
  },
  async getTaskLog(): Promise<string> {
    return mockLogSample;
  },
  /**
   * 假端口占用：mock 服务的 3001 报成被外部 node.exe 占着，
   * 用来预览「已退出 + 外部运行中」这种状态；5173 归 bootpane 自己管，不重复标记。
   */
  async checkPorts(ports: number[]) {
    return ports.map((port) => {
      const hit = mockOccupied[port];
      return {
        port,
        listening: !!hit,
        pid: hit?.pid ?? 0,
        process: hit?.process ?? "",
      };
    });
  },
  /** 假重启：把被占的 3001 让出来，然后当成自己启动的实例。 */
  async restartTask(projectId: string, taskId: string) {
    delete mockOccupied[3001];
    mockRuntimes.set(`${projectId}/${taskId}`, {
      projectId,
      taskId,
      state: "running",
      pid: 31337,
      ready: true,
      startedAt: new Date().toISOString(),
      exitCode: 0,
      cpuPercent: 4.1,
      memoryBytes: 96 * 1024 * 1024,
      processes: 2,
    });
    return {
      ok: true,
      killed: ["node.exe (PID 47772)"],
      message: "已终止外部实例并重新启动",
    };
  },
};
