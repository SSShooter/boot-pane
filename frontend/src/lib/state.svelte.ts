import type { ResolvedProject } from "../../bindings/bootpane/internal/config/models";
import type { Runtime } from "../../bindings/bootpane/internal/process/models";
import type { Editor } from "../../bindings/bootpane/internal/editors/models";
import type { Listener } from "../../bindings/bootpane/internal/portinfo/models";
import { Events } from "@wailsio/runtime";
import { api, isPreview } from "./api";
import { EVENT_CLOSE_REQUEST, EVENT_LOG, EVENT_RUNTIME, EVENT_STATS, taskKey } from "./events";
import type { CloseRequestPayload, LogPayload, StatsPayload } from "./events";
import { MAX_LOG_CHARS } from "./format";
import { openWithLabel } from "./openwith";
import { countRuns, createAnsiState, mergeRuns, parseAnsiChunk, trimRuns } from "./ansi";
import type { AnsiState, LogRun } from "./ansi";

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

/**
 * 设置页的伪项目 id。
 *
 * 和 STARRED_VIEW 同理：设置页是主区的一整页，不是弹窗，
 * 所以也得借 selectedId 表达「当前主区显示什么」。
 */
export const SETTINGS_VIEW = "__settings__";

/** 不对应真实项目的视图 id。 */
function isPseudoView(id: string): boolean {
  return id === STARRED_VIEW || id === SETTINGS_VIEW;
}

/**
 * 一个待用户确认的操作。
 *
 * 用结构化的字段而不是一整段纯文本：标题、正文、补充说明在界面上
 * 的字号和颜色都不一样，塞进一个字符串里就没法分层了。
 */
export type ConfirmRequest = {
  /** 一句话说清要做什么，作为弹窗标题。 */
  title: string;
  /** 主内容：动作本身和受影响的对象。 */
  body?: string;
  /**
   * 需要逐行列出的对象（比如正在运行的任务）。
   *
   * 塞进 body 里用顿号串成一句会很难扫，尤其是项目名长、任务多的时候。
   */
  items?: string[];
  /** 补充说明：后果、不可逆提示之类，视觉上弱化。 */
  detail?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  /** 破坏性操作用红色确认按钮。 */
  danger?: boolean;
};

/** 一个任务的日志：带样式的片段 + 当前字符数（裁剪时要判断，别每次重算）。 */
type LogBuffer = { runs: LogRun[]; chars: number };

/** 空日志的稳定引用，避免每次派生都造新数组把渲染搅乱。 */
const EMPTY_RUNS: LogRun[] = [];

class AppState {
  projects = $state<ResolvedProject[]>([]);
  runtimes = $state<Record<string, Runtime>>({});
  /**
   * 每个任务已累积的日志片段，键是 `projectId/taskId`。
   *
   * 用 raw 而不是深响应：片段数组只整体替换，深代理几万个节点得不偿失。
   */
  logs = $state.raw<Record<string, LogBuffer>>({});
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

  /**
   * 本机偏好，镜像 workspace.json 顶层的那份数据。
   *
   * 只有「这台机器上的习惯」放这里（星标、自动打开日志、用哪个编辑器开目录），
   * 项目级的配置永远走 workspace.json 里的项目条目或仓库内 .bootpane.json。
   */
  settings = $state({ autoOpenLogOnStart: false, openWith: "" });

  /**
   * 本机扫到的编辑器，设置页的「打开方式」只列这些。
   *
   * 空数组既表示「还没扫完」也表示「一个都没装」，两种情况下拉框都退回
   * 内置预设——用户在装完编辑器之前，至少还能看到 VS Code 这类常见选项。
   */
  editors = $state<Editor[]>([]);

  /** 当前「打开目录」用的命令，空串表示系统文件管理器。 */
  openWith = $derived((this.settings.openWith ?? "").trim());

  /**
   * 「打开目录」按钮上的目标名，比如「VS Code」。
   *
   * 先按扫描结果查：绝对路径形式的命令（`"C:\...\Trae.exe"`）从文件名
   * 只能认出 `Trae.exe`，而扫描结果里存着完整名字。
   */
  openWithLabel = $derived.by(() => {
    const command = this.openWith;
    if (!command) return "系统文件管理器";
    return this.editors.find((e) => e.command === command)?.name ?? openWithLabel(command);
  });

  /** 当前打开「生成配置」弹窗的项目 id，null 表示弹窗关闭。 */
  generatorProjectId = $state<string | null>(null);

  /** 当前打开「编辑配置」弹窗的项目 id，null 表示弹窗关闭。 */
  editorProjectId = $state<string | null>(null);

  toast = $state("");
  toastIsError = $state(false);

  /**
   * 当前弹出的应用内确认框，null 表示没弹。
   *
   * 不用 window.confirm：原生弹窗画在窗口之外（Windows 上是系统对话框），
   * 主题、字体、按钮都跟界面完全脱节，还会阻塞渲染线程。
   */
  confirmRequest = $state<ConfirmRequest | null>(null);

  private confirmResolve: ((ok: boolean) => void) | null = null;
  private toastTimer: ReturnType<typeof setTimeout> | undefined;
  /** 每个任务的 ANSI 解析状态（当前颜色、未收完的转义序列尾巴）。 */
  private ansiStates = new Map<string, AnsiState>();

  selected = $derived(this.projects.find((p) => p.id === this.selectedId) ?? null);

  /** 当前是不是停在星标聚合视图。 */
  showStarred = $derived(this.selectedId === STARRED_VIEW);

  /** 当前是不是停在设置页。 */
  showSettings = $derived(this.selectedId === SETTINGS_VIEW);

  /**
   * 星标任务的跨项目扁平列表。
   *
   * 顺序跟着 projects 走（置顶 + 名称），组内保持配置里的任务顺序——
   * 这样聚合视图里的相对次序和项目面板一致，不至于找不到卡片。
   */
  starredTasks = $derived(
    this.projects.flatMap((project) =>
      (project.tasks ?? [])
        .filter((task) => (project.starredTaskIds ?? []).includes(task.id))
        .map((task) => ({ project, task })),
    ),
  );

  generatorProject = $derived(
    this.projects.find((p) => p.id === this.generatorProjectId) ?? null,
  );

  editorProject = $derived(
    this.projects.find((p) => p.id === this.editorProjectId) ?? null,
  );

  /** 是否有确认框正开着（键盘处理要据此让路，否则 Esc 会被抢两次）。 */
  confirmOpen = $derived(this.confirmRequest !== null);

  activeLogKey = $derived(
    this.activeLog ? taskKey(this.activeLog.projectId, this.activeLog.taskId) : "",
  );

  /** 当前日志面板要渲染的片段。 */
  activeLogRuns = $derived(
    this.activeLogKey ? (this.logs[this.activeLogKey]?.runs ?? EMPTY_RUNS) : EMPTY_RUNS,
  );

  /** 拉取项目列表，并在必要时修正当前选中项。 */
  async load(): Promise<void> {
    this.loading = true;
    try {
      const list = await api.listProjects();
      this.projects = list ?? [];
      // 伪视图（星标 / 设置）不是真实项目，不能被「选中项不存在」的兜底逻辑
      // 踢回第一个项目。
      if (!isPseudoView(this.selectedId) && !this.projects.some((p) => p.id === this.selectedId)) {
        this.selectedId = this.projects[0]?.id ?? "";
      }
      // 编辑器扫描要走几十次 PATH 查找和目录读取，不等它：设置页是低频入口，
      // 扫完之后列表自己会刷新，没必要为了它拖慢首屏。
      void this.detectEditors();

      await Promise.all([this.refreshRuntimes(), this.refreshPorts(), this.loadSettings()]);
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
        Events.On(EVENT_CLOSE_REQUEST, (ev) => {
          void this.handleCloseRequest(ev.data as CloseRequestPayload);
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

  /**
   * 用户关窗口时后端发现还有任务在跑，把决定权交给这里。
   *
   * 先回执再弹框：后端等不到回执会按「直接退出」兜底，那条路径是给
   * webview 挂掉准备的，正常流程必须在弹框前就把回执发出去。
   */
  private async handleCloseRequest(payload: CloseRequestPayload): Promise<void> {
    void api.acknowledgeClose().catch(() => {});

    const tasks = payload?.tasks ?? [];
    const names = tasks
      .map((t) => this.describeTask(t.projectId, t.taskId))
      .filter((name) => name !== "");

    const ok = await this.confirm({
      title: "还有任务在运行",
      body:
        names.length > 0
          ? `关闭会一并停止以下 ${names.length} 个任务：`
          : `关闭会一并停止 ${tasks.length} 个正在运行的任务。`,
      items: names,
      detail: "进程会被强制终止，没来得及落盘的构建产物或写入可能不完整。",
      confirmLabel: "停止并退出",
      cancelLabel: "取消",
      danger: true,
    });

    try {
      if (ok) await api.confirmQuit();
      else await api.cancelQuit();
    } catch {
      // 确认退出时后端可能已经把进程收掉了，这条报错没有展示的意义。
    }
  }

  /** 把任务 id 翻成「项目 · 任务」；配置里找不到就返回空串，由调用方兜底。 */
  private describeTask(projectId: string, taskId: string): string {
    const project = this.projects.find((p) => p.id === projectId);
    const task = (project?.tasks ?? []).find((t) => t.id === taskId);
    if (!project || !task) return "";
    return `${project.name} · ${task.label}`;
  }

  /** 拉一次本机偏好。失败不打扰用户，停在默认值即可。 */
  async loadSettings(): Promise<void> {
    try {
      const st = await api.getSettings();
      this.settings = {
        autoOpenLogOnStart: !!st?.autoOpenLogOnStart,
        openWith: st?.openWith ?? "",
      };
    } catch {
      // 读不到一条偏好不值得弹错误，用默认值继续。
    }
  }

  /**
   * 扫一遍本机装了哪些编辑器。
   *
   * 失败就当没扫到：设置页还能手填命令，不值得为一次探测弹错误。
   */
  async detectEditors(): Promise<void> {
    try {
      this.editors = (await api.detectEditors()) ?? [];
    } catch {
      this.editors = [];
    }
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
      for (const task of project.tasks ?? []) {
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
   * 取任务的 ANSI 解析状态。
   *
   * 状态挂在任务上而不是每块新建：颜色会跨数据块延续，未写完的转义序列
   * 也要留到下一块才能接着解析。
   */
  private ansiFor(key: string): AnsiState {
    let state = this.ansiStates.get(key);
    if (!state) {
      state = createAnsiState();
      this.ansiStates.set(key, state);
    }
    return state;
  }

  /** 追加一段日志：解析颜色、跨块合并、超上限时从头部裁掉。 */
  private appendLog(key: string, chunk: string): void {
    if (!chunk) return;

    const fresh = parseAnsiChunk(this.ansiFor(key), chunk);
    if (fresh.length === 0) return;

    const prev = this.logs[key];
    const runs = mergeRuns(prev?.runs ?? EMPTY_RUNS, fresh);
    const chars = (prev?.chars ?? 0) + countRuns(fresh);
    this.logs = { ...this.logs, [key]: trimRuns(runs, chars, MAX_LOG_CHARS) };
  }

  /** 整体替换某个任务的日志（从后端补一次历史输出时用）。 */
  private replaceLog(key: string, runs: LogRun[]): void {
    const chars = countRuns(runs);
    this.logs = { ...this.logs, [key]: trimRuns(runs, chars, MAX_LOG_CHARS) };
  }

  /** 丢掉一个任务的日志和解析状态。清空、重启、换任务都走这里。 */
  private resetLog(key: string): void {
    this.ansiStates.delete(key);
    if (!this.logs[key]) return;

    const next = { ...this.logs };
    delete next[key];
    this.logs = next;
  }

  select(id: string): void {
    this.selectedId = id;
  }

  /** 打开设置页。和项目共用 selectedId，靠伪 id 区分。 */
  openSettings(): void {
    this.selectedId = SETTINGS_VIEW;
  }

  /**
   * 切换「启动任务后自动打开日志」。
   *
   * 先本地生效再回写：开关等一次 IPC 往返才动，点起来像没反应。
   * 写盘失败就回滚并提示——偏好没存住这件事必须让用户知道，
   * 否则下次启动行为不一样他会以为是自己记错了。
   */
  async setAutoOpenLogOnStart(enabled: boolean): Promise<void> {
    const prev = this.settings.autoOpenLogOnStart;
    if (prev === enabled) return;

    this.settings = { ...this.settings, autoOpenLogOnStart: enabled };
    try {
      await api.setAutoOpenLogOnStart(enabled);
    } catch (e) {
      this.settings = { ...this.settings, autoOpenLogOnStart: prev };
      this.notify(describeError(e), true);
    }
  }

  /**
   * 切换「打开目录」用的命令。空串表示回到系统文件管理器。
   *
   * 和自动打开日志一样先本地生效再回写：命令拼错时后端会拒绝，
   * 这时回滚并提示，用户不用去点一次「打开目录」才发现设置没生效。
   */
  async setOpenWith(command: string): Promise<void> {
    const next = command.trim();
    const prev = this.settings.openWith;
    if (prev === next) return;

    this.settings = { ...this.settings, openWith: next };
    try {
      await api.setOpenWith(next);
    } catch (e) {
      this.settings = { ...this.settings, openWith: prev };
      this.notify(describeError(e), true);
    }
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
      if (!(await this.startTask(project.id, task.id, false))) failed++;
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
    if (this.logs[key]?.chars) return;
    try {
      const text = await api.getTaskLog(projectId, taskId);
      if (!text || this.logs[key]?.chars) return;
      // 后端缓冲里同样是带 ANSI 的原始字节流，必须走同一套解析，
      // 否则「刚打开面板」和「跑了半天」看到的是两种格式。
      this.resetLog(key);
      this.replaceLog(key, parseAnsiChunk(this.ansiFor(key), text));
    } catch {
      // 补不到就算了，后续事件来了照样能显示。
    }
  }

  closeLog(): void {
    this.activeLog = null;
  }

  clearLog(): void {
    if (!this.activeLog) return;
    this.resetLog(taskKey(this.activeLog.projectId, this.activeLog.taskId));
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

  /**
   * 任务对应的页面地址，空串表示这个任务没有可打开的页面。
   *
   * 地址由 Go 侧统一解析（显式 openUrl 或前端任务按端口兜底）后随项目下发，
   * 前端不重复实现一套判定，免得多出一个按钮点不开。
   */
  pageUrlOf(projectId: string, taskId: string): string {
    const p = this.projects.find((x) => x.id === projectId);
    return p?.pageUrls?.[taskId] ?? "";
  }

  async openTaskPage(projectId: string, taskId: string): Promise<void> {
    try {
      await api.openTaskPage(projectId, taskId);
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

  openEditor(id: string): void {
    this.editorProjectId = id;
  }

  closeEditor(): void {
    this.editorProjectId = null;
  }

  /**
   * 启动任务，返回是否成功（批量启动要据此统计失败数）。
   *
   * autoLog 控制「启动后自动展开日志」这条偏好。批量启动传 false：
   * 连着弹几个任务的面板只会让它来回跳，用户要的是「都起来了吗」，
   * 不是逐个看日志。
   */
  async startTask(projectId: string, taskId: string, autoLog = true): Promise<boolean> {
    const key = taskKey(projectId, taskId);
    // 重新启动意味着新一轮输出，旧的留着只会让人误判。
    this.resetLog(key);

    try {
      await api.startTask(projectId, taskId);
      await this.refreshRuntimes();

      // 已经开着别的任务的面板时不抢占：用户正看着日志，被切走比不弹更烦。
      if (autoLog && this.settings.autoOpenLogOnStart && !this.activeLog) {
        void this.openLog(projectId, taskId);
      }
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
    this.resetLog(key);
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

  /**
   * 弹一个应用内确认框，等用户点按钮后兑现 true / false。
   *
   * 上一个还没答完（正常流程走不到）就先按取消兑现，免得那个 promise 永远悬着。
   */
  confirm(req: ConfirmRequest): Promise<boolean> {
    this.resolveConfirm(false);
    return new Promise<boolean>((resolve) => {
      this.confirmResolve = resolve;
      this.confirmRequest = req;
    });
  }

  /** 关掉确认框并把结果交给等待方。 */
  resolveConfirm(ok: boolean): void {
    this.confirmRequest = null;
    const resolve = this.confirmResolve;
    this.confirmResolve = null;
    resolve?.(ok);
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
