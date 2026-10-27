import type { ResolvedProject } from "../../bindings/bootpane/internal/config/models";
import type { Runtime } from "../../bindings/bootpane/internal/process/models";
import type { Listener } from "../../bindings/bootpane/internal/portinfo/models";
import { Events } from "@wailsio/runtime";
import { api, isPreview } from "./api";
import { EVENT_LOG, EVENT_RUNTIME, EVENT_STATS, taskKey } from "./events";
import type { LogPayload, StatsPayload } from "./events";
import { MAX_LOG_CHARS, collapseCarriageReturns, stripAnsi } from "./format";

/** 把 Go 侧返回的 error 转成能直接展示的文案。 */
function describeError(e: unknown): string {
  if (typeof e === "string") return e;
  if (e instanceof Error) return e.message;
  if (e && typeof e === "object" && "message" in e) return String((e as any).message);
  return String(e);
}

/**
 * 兜底对账间隔。
 *
 * 正常状态下状态全靠事件推送，这个定时拉取只防一种情况：窗口重载后
 * 错过了之前的事件，列表会停在旧状态。30s 足够低频，不会造成负担。
 */
const RECONCILE_INTERVAL = 30_000;

/**
 * 端口占用轮询间隔。
 *
 * 端口状态没法靠事件推——被占用的可能是完全不受 bootpane 管的进程，
 * 没有任何事件会通知我们它起来了或退了。5s 一次 GetExtendedTcpTable
 * 是一次系统调用，开销可以忽略。
 */
const PORT_POLL_INTERVAL = 5_000;

/**
 * 「星标任务」聚合视图的伪项目 id。
 *
 * 侧栏里它是一个固定条目，选中后主区显示跨项目的星标卡片。
 * 用保留字符串而不是再造一个真实项目，是因为它不对应任何磁盘目录。
 */
export const STARRED_VIEW = "__starred__";

class AppState {
  projects = $state<ResolvedProject[]>([]);
  runtimes = $state<Record<string, Runtime>>({});
  /** 每个任务已累积的日志文本，键是 `projectId/taskId`。 */
  logs = $state<Record<string, string>>({});
  /**
   * 端口占用情况，键是端口号。
   *
   * 和 runtimes 是两回事：runtimes 说的是「bootpane 启动的进程还在不在」，
   * 这里说的是「这个端口现在有没有人监听」。启动脚本自己判断「已在运行」
   * 就退出时，两者会不一致，界面必须如实分别表达。
   */
  ports = $state<Record<number, Listener>>({});
  /**
   * 正在重启的任务，键是 `projectId/taskId`。
   *
   * 重启要等端口真正释放（TerminateProcess 是异步的），后端会阻塞几秒，
   * 这段时间按钮必须锁住，否则用户连点会反复杀进程。
   */
  restarting = $state<Record<string, boolean>>({});
  /** 当前打开日志面板的任务，null 表示面板关闭。 */
  activeLog = $state<{ projectId: string; taskId: string } | null>(null);

  selectedId = $state("");
  loading = $state(true);

  /** 当前打开「生成配置」弹窗的项目 id，null 表示弹窗关闭。 */
  generatorProjectId = $state<string | null>(null);

  toast = $state("");
  toastIsError = $state(false);

  private toastTimer: ReturnType<typeof setTimeout> | undefined;
  /** 每个任务尚未收完的 ANSI 转义序列尾巴，跨事件块续接用。 */
  private ansiCarry = new Map<string, string>();

  selected = $derived(this.projects.find((p) => p.id === this.selectedId) ?? null);

  /** 当前是不是停在星标聚合视图。 */
  showStarred = $derived(this.selectedId === STARRED_VIEW);

  /**
   * 星标任务的跨项目扁平列表。
   *
   * 顺序跟着 projects 走（置顶 + 名称），组内保持配置里的任务顺序——
   * 这样聚合视图里的相对次序和项目面板一致，不至于找不到卡片。
   */
  starredTasks = $derived(
    this.projects.flatMap((project) =>
      project.tasks
        .filter((task) => (project.starredTaskIds ?? []).includes(task.id))
        .map((task) => ({ project, task })),
    ),
  );

  generatorProject = $derived(
    this.projects.find((p) => p.id === this.generatorProjectId) ?? null,
  );

  activeLogKey = $derived(
    this.activeLog ? taskKey(this.activeLog.projectId, this.activeLog.taskId) : "",
  );

  /** 当前日志面板要显示的文本。 */
  activeLogText = $derived(this.activeLogKey ? (this.logs[this.activeLogKey] ?? "") : "");

  /** 拉取项目列表，并在必要时修正当前选中项。 */
  async load(): Promise<void> {
    this.loading = true;
    try {
      const list = await api.listProjects();
      this.projects = list ?? [];
      // 星标视图不是真实项目，不能被「选中项不存在」的兜底逻辑踢回第一个项目。
      if (
        this.selectedId !== STARRED_VIEW &&
        !this.projects.some((p) => p.id === this.selectedId)
      ) {
        this.selectedId = this.projects[0]?.id ?? "";
      }
      await Promise.all([this.refreshRuntimes(), this.refreshPorts()]);
    } catch (e) {
      this.notify(describeError(e), true);
    } finally {
      this.loading = false;
    }
  }

  /**
   * 订阅后端事件并启动轮询，返回取消函数。
   *
   * 浏览器预览模式下没有 Wails 运行时，事件订阅要跳过（Events.On 会抛错），
   * 但轮询照跑——mock 数据同样能让界面显示端口占用。
   */
  connect(): () => void {
    const offs: (() => void)[] = [];
    const timers: ReturnType<typeof setInterval>[] = [];

    if (!isPreview) {
      offs.push(
        Events.On(EVENT_RUNTIME, (ev) => {
          const rt = ev.data as Runtime;
          if (!rt?.projectId) return;
          this.runtimes[taskKey(rt.projectId, rt.taskId)] = rt;
        }),
        Events.On(EVENT_LOG, (ev) => {
          const p = ev.data as LogPayload;
          if (!p?.projectId) return;
          this.appendLog(taskKey(p.projectId, p.taskId), p.chunk);
        }),
        Events.On(EVENT_STATS, (ev) => {
          const p = ev.data as StatsPayload;
          if (!p?.projectId) return;
          const key = taskKey(p.projectId, p.taskId);
          const cur = this.runtimes[key];
          if (!cur) return;
          // 只更新资源字段，避免整份快照替换把状态字段覆盖成旧的。
          this.runtimes[key] = {
            ...cur,
            cpuPercent: p.cpuPercent,
            memoryBytes: p.memoryBytes,
            processes: p.processes,
          };
        }),
      );
    }

    timers.push(setInterval(() => void this.refreshRuntimes(), RECONCILE_INTERVAL));
    timers.push(setInterval(() => void this.refreshPorts(), PORT_POLL_INTERVAL));

    return () => {
      for (const off of offs) off();
      for (const t of timers) clearInterval(t);
    };
  }

  /** 拉一次全量运行状态，用于首次加载和兜底对账。 */
  async refreshRuntimes(): Promise<void> {
    try {
      const list = await api.listRunning();
      const next: Record<string, Runtime> = {};
      for (const r of list ?? []) {
        next[taskKey(r.projectId, r.taskId)] = r;
      }
      this.runtimes = next;
    } catch {
      // 进程服务不可用时不打扰用户，界面停在空状态即可。
    }
  }

  /**
   * 批量刷新端口占用。
   *
   * 只查当前项目里配置了 `waitFor.port` 的端口——没配端口的任务
   * （比如纯编译任务）本来就没有「服务在不在」这个概念。
   */
  async refreshPorts(): Promise<void> {
    const wanted = new Set<number>();
    for (const project of this.projects) {
      for (const task of project.tasks) {
        const port = task.waitFor?.type === "port" ? (task.waitFor.port ?? 0) : 0;
        if (port > 0) wanted.add(port);
      }
    }

    if (wanted.size === 0) {
      if (Object.keys(this.ports).length > 0) this.ports = {};
      return;
    }

    try {
      const list = await api.checkPorts([...wanted]);
      const next: Record<number, Listener> = {};
      for (const l of list ?? []) next[l.port] = l;
      this.ports = next;
    } catch {
      // 查不到就当不知道，别用空结果覆盖掉已有信息。
    }
  }

  /** 端口当前是否被占用（不管是谁占的）。 */
  portOf(task: { waitFor?: { type?: string; port?: number } | null }): Listener | null {
    if (task.waitFor?.type !== "port") return null;
    const port = task.waitFor.port ?? 0;
    return port > 0 ? (this.ports[port] ?? null) : null;
  }

  /**
   * 把原始终端字节流净化成可显示的文本。
   *
   * 三步缺一不可：剥掉 ANSI（否则满屏 \x1b[32m）、压掉行内回车
   * （否则 spinner 会刷出几十行）、再由调用方按上限裁剪。
   */
  private sanitize(key: string, chunk: string): string {
    const carry = this.ansiCarry.get(key) ?? "";
    const { clean, carry: rest } = stripAnsi(carry + chunk);
    this.ansiCarry.set(key, rest);
    return collapseCarriageReturns(clean);
  }

  /** 追加一段日志。 */
  private appendLog(key: string, chunk: string): void {
    if (!chunk) return;

    const text = this.sanitize(key, chunk);
    if (!text) return;

    let next = (this.logs[key] ?? "") + text;
    if (next.length > MAX_LOG_CHARS) {
      // 从换行处切，别把一行切成两半。
      const cut = next.length - MAX_LOG_CHARS;
      const nl = next.indexOf("\n", cut);
      next = next.slice(nl >= 0 ? nl + 1 : cut);
    }
    this.logs[key] = next;
  }

  select(id: string): void {
    this.selectedId = id;
  }

  runtimeOf(projectId: string, taskId: string): Runtime | null {
    return this.runtimes[taskKey(projectId, taskId)] ?? null;
  }

  /** 任务是否正在跑（含启动中）。批量操作要靠它跳过已启动的。 */
  isBusy(projectId: string, taskId: string): boolean {
    const st = this.runtimeOf(projectId, taskId)?.state;
    return st === "running" || st === "starting";
  }

  isStarred(projectId: string, taskId: string): boolean {
    const p = this.projects.find((x) => x.id === projectId);
    return (p?.starredTaskIds ?? []).includes(taskId);
  }

  /**
   * 切换星标。
   *
   * 只改本地那一份 starredTaskIds，不整表重拉：重拉会让 projects 换新对象，
   * 正在展开的日志面板和滚动位置都会跟着抖一下，点个星标不值当。
   */
  async toggleStar(projectId: string, taskId: string): Promise<void> {
    const p = this.projects.find((x) => x.id === projectId);
    if (!p) return;

    const current = p.starredTaskIds ?? [];
    const starred = !current.includes(taskId);
    try {
      await api.setStar(projectId, taskId, starred);
      p.starredTaskIds = starred
        ? [...current, taskId]
        : current.filter((id) => id !== taskId);
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  /**
   * 依次启动所有星标任务。
   *
   * 串行而不是并行：同时拉起一堆 dev server 会互相抢端口和 CPU，
   * 失败时也分不清是哪一条炸的。
   */
  async startStarred(): Promise<void> {
    const targets = this.starredTasks.filter(({ project, task }) => !this.isBusy(project.id, task.id));
    if (targets.length === 0) {
      this.notify("星标任务都已经在运行");
      return;
    }

    let failed = 0;
    for (const { project, task } of targets) {
      if (!(await this.startTask(project.id, task.id))) failed++;
    }
    await this.refreshPorts();

    const ok = targets.length - failed;
    this.notify(
      failed > 0 ? `已启动 ${ok} 个，${failed} 个失败` : `已依次启动 ${ok} 个星标任务`,
      failed > 0,
    );
  }

  /** 停止所有正在运行的星标任务。 */
  async stopStarred(): Promise<void> {
    const targets = this.starredTasks.filter(({ project, task }) => this.isBusy(project.id, task.id));
    if (targets.length === 0) {
      this.notify("没有正在运行的星标任务");
      return;
    }

    for (const { project, task } of targets) {
      await this.stopTask(project.id, task.id);
    }
    await this.refreshPorts();
    this.notify(`已停止 ${targets.length} 个星标任务`);
  }

  /** 打开某个任务的日志面板。 */
  async openLog(projectId: string, taskId: string): Promise<void> {
    this.activeLog = { projectId, taskId };
    const key = taskKey(projectId, taskId);

    // 前端没攒到内容（比如窗口刚重载）时，从后端环形缓冲补一次历史。
    if (this.logs[key]) return;
    try {
      const text = await api.getTaskLog(projectId, taskId);
      if (!text || this.logs[key]) return;
      // 后端缓冲里同样是带 ANSI 的原始字节流，必须走同一套净化，
      // 否则「刚打开面板」和「跑了半天」看到的是两种格式。
      this.ansiCarry.delete(key);
      this.logs[key] = this.sanitize(key, text);
    } catch {
      // 补不到就算了，后续事件来了照样能显示。
    }
  }

  closeLog(): void {
    this.activeLog = null;
  }

  clearLog(): void {
    if (!this.activeLog) return;
    const key = taskKey(this.activeLog.projectId, this.activeLog.taskId);
    this.logs[key] = "";
    this.ansiCarry.delete(key);
  }

  async addProject(path: string): Promise<void> {
    if (!path) return;
    try {
      const added = await api.addProject(path);
      await this.load();
      this.selectedId = added.id;
      this.notify(`已添加 ${added.name}`);
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  async removeProject(id: string): Promise<void> {
    try {
      await api.removeProject(id);
      if (this.selectedId === id) this.selectedId = "";
      if (this.activeLog?.projectId === id) this.closeLog();
      await this.load();
      this.notify("已从列表移除（磁盘文件未受影响）");
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  async openProjectDir(id: string): Promise<void> {
    try {
      await api.openProjectDir(id);
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  async openConfigDir(): Promise<void> {
    try {
      await api.openConfigDir();
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  /** 切换配置来源：全局注册表 ↔ 仓库内 .bootpane.json。 */
  async setUseRepoConfig(id: string, enabled: boolean): Promise<void> {
    try {
      await api.setUseRepoConfig(id, enabled);
      await this.load();
      this.notify(enabled ? "已切换为项目内配置" : "已切换为全局配置");
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  openGenerator(id: string): void {
    this.generatorProjectId = id;
  }

  closeGenerator(): void {
    this.generatorProjectId = null;
  }

  /** 启动任务，返回是否成功（批量启动要据此统计失败数）。 */
  async startTask(projectId: string, taskId: string): Promise<boolean> {
    const key = taskKey(projectId, taskId);
    // 重新启动意味着新一轮输出，旧的留着只会让人误判。
    this.logs[key] = "";
    this.ansiCarry.delete(key);

    try {
      await api.startTask(projectId, taskId);
      await this.refreshRuntimes();
      return true;
    } catch (e) {
      this.notify(describeError(e), true);
      return false;
    }
  }

  async stopTask(projectId: string, taskId: string): Promise<void> {
    try {
      await api.stopTask(projectId, taskId);
      await this.refreshRuntimes();
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  /**
   * 重启任务：端口被外部实例占着时，先杀掉整棵进程树再启动。
   *
   * 只用于「外部运行中」的场景。正常停止的任务直接走 startTask 就行，
   * 没必要多绕一层杀进程。
   */
  async restartTask(projectId: string, taskId: string): Promise<void> {
    const key = taskKey(projectId, taskId);
    if (this.restarting[key]) return;

    this.restarting[key] = true;
    // 上一轮的日志会被新实例的输出取代，先清掉。
    this.logs[key] = "";
    this.ansiCarry.delete(key);
    this.notify("正在终止外部实例…");

    try {
      const res = await api.restartTask(projectId, taskId);
      if (res?.ok) {
        const n = res.killed?.length ?? 0;
        this.notify(n > 0 ? `已终止 ${n} 个进程，正在重启` : "已重启");
      } else {
        this.notify(res?.message || "重启失败", true);
      }
    } catch (e) {
      this.notify(describeError(e), true);
    } finally {
      this.restarting[key] = false;
      await Promise.all([this.refreshRuntimes(), this.refreshPorts()]);
    }
  }

  notify(message: string, isError = false): void {
    this.toast = message;
    this.toastIsError = isError;
    clearTimeout(this.toastTimer);
    this.toastTimer = setTimeout(
      () => {
        this.toast = "";
      },
      isError ? 7000 : 2600,
    );
  }
}

export const app = new AppState();
