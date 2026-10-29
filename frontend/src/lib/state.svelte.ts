import type { ResolvedProject } from "../../bindings/bootpane/internal/config/models";
import type { Runtime } from "../../bindings/bootpane/internal/process/models";
import type { Editor } from "../../bindings/bootpane/internal/editors/models";
import type { Listener } from "../../bindings/bootpane/internal/portinfo/models";
import { Events } from "@wailsio/runtime";
import { api, isPreview } from "./api";
import {
  EVENT_CLOSE_REQUEST,
  EVENT_LOG,
  EVENT_LOG_CLEAR,
  EVENT_LOG_EMBED,
  EVENT_LOG_STATE,
  EVENT_LOG_STATE_ASK,
  EVENT_LOG_WINDOW_CLOSED,
  EVENT_RUNTIME,
  EVENT_STATS,
  LOG_WINDOW_QUERY,
  taskKey,
} from "./events";
import type {
  CloseRequestPayload,
  LogClearPayload,
  LogPayload,
  LogStatePayload,
  StatsPayload,
} from "./events";
import { MAX_LOG_CHARS } from "./format";
import { openWithLabel } from "./openwith";
import { countRuns, createAnsiState, mergeRuns, parseAnsiChunk, trimRuns } from "./ansi";
import type { AnsiState, LogRun } from "./ansi";
import { closeCurrentWindow } from "./window";

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
 * 当前窗口的随机标识。
 *
 * 跨窗口同步走的是广播事件，自己也会收到自己发的那条；带上它才能把回音
 * 认出来忽略掉，不然每次操作都会在原地再采纳一遍。
 */
const WINDOW_ID = Math.random().toString(36).slice(2, 10);

/**
 * 解析独立窗口入口地址里的面板状态。
 *
 * 地址是外部输入，解析失败就返回 null 而不是抛：独立窗口退化成空面板，
 * 用户还能点「内嵌」回去，而不是对着一片白屏不知所措。
 */
function parseLogSnapshot(raw: string): LogStatePayload | null {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed?.tabs)) return null;
    const tabs = parsed.tabs
      .filter((t: any) => t && t.key && t.projectId && t.taskId)
      .map((t: any) => ({
        key: String(t.key),
        projectId: String(t.projectId),
        taskId: String(t.taskId),
      }));
    return { source: String(parsed.source ?? ""), key: String(parsed.key ?? ""), tabs };
  } catch {
    return null;
  }
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

/**
 * 一个日志标签页。
 *
 * 只记位置不记内容：日志本体在 logs 里，标签说清「看的是哪个任务的日志」。
 * key 存下来是因为它要参与比较（激活、未读），每次现拼不划算。
 */
export type LogTab = { key: string; projectId: string; taskId: string };

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
  /**
   * 打开过的日志标签，顺序即标签条的显示顺序。
   *
   * 只有用户主动关标签才会移除。启动新任务时新标签顶到最前看着，
   * 旧标签留着——对照几个应用的输出是这类工具最常见的用法。
   */
  logTabs = $state<LogTab[]>([]);
  /** 当前选中的标签 key（`projectId/taskId`）。收起抽屉时保留，展开能回到原位。 */
  logKey = $state("");
  /** 日志抽屉是否展开。有标签但收起时，侧栏的「日志」入口能再打开。 */
  logOpen = $state(false);
  /**
   * 日志面板是否已经弹成独立窗口。
   *
   * 弹出后主窗口不再渲染内嵌面板——同一块面板只能出现在一个地方，否则
   * 两边各显示一半状态，用户不知道该看哪个。
   */
  logDetached = $state(false);
  /**
   * 不是当前标签的任务又输出了内容。
   *
   * 没有这个标记，切过去才发现前后两段接不上；有了它，标签上的小点
   * 能告诉用户「这条后面还有没看的东西」。
   */
  logUnread = $state<Record<string, boolean>>({});

  selectedId = $state("");
  loading = $state(true);

  /**
   * 当前窗口的角色。
   *
   * `main` 带侧栏和项目面板，`log` 只承载日志面板。两边跑的是同一份代码，
   * 但关闭确认、收回面板这些动作只归主窗口管。
   */
  private role: "main" | "log" = "main";

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

  /** 当前标签对应的任务；抽屉收起时为 null，面板据此整块不渲染。 */
  activeLog = $derived.by(() => {
    if (!this.logOpen || !this.logKey) return null;
    return this.logTabs.find((t) => t.key === this.logKey) ?? null;
  });

  activeLogKey = $derived(this.activeLog?.key ?? "");

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
   *
   * role 决定这份实例是不是主窗口：关窗确认弹在内嵌窗口里没有任何意义，
   * 独立日志窗口跟着弹一遍只会让用户不知道该点哪个。
   */
  connect(role: "main" | "log" = "main"): () => void {
    this.role = role;
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
        Events.On(EVENT_LOG_STATE, (ev) => {
          this.adoptLogState(ev.data as LogStatePayload);
        }),
        Events.On(EVENT_LOG_STATE_ASK, (ev) => {
          if ((ev.data as { source?: string })?.source === WINDOW_ID) return;
          this.syncLogState();
        }),
        Events.On(EVENT_LOG_CLEAR, (ev) => {
          const p = ev.data as LogClearPayload;
          if (!p?.key || p.source === WINDOW_ID) return;
          this.resetLog(p.key);
        }),
      );

      if (role === "main") {
        offs.push(
          Events.On(EVENT_CLOSE_REQUEST, (ev) => {
            void this.handleCloseRequest(ev.data as CloseRequestPayload);
          }),
          // 独立窗口没了，不管是点了「内嵌」还是直接叉掉。
          Events.On(EVENT_LOG_EMBED, () => this.embedLogPanel()),
          Events.On(EVENT_LOG_WINDOW_CLOSED, () => this.embedLogPanel()),
        );
      } else {
        // 独立窗口刚起来就问一句：URL 里的快照是弹窗那一刻的，之后主窗口
        // 可能又开了新标签。
        void Events.Emit(EVENT_LOG_STATE_ASK, { source: WINDOW_ID });
      }
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

    // 后台标签有新输出就打个未读标记。只对开着的标签做——没标签的任务
    // 没人会去看，标了也没地方显示。抽屉收起时当前标签同样算「看不着」。
    if (!this.logUnread[key] && (!this.logOpen || this.logKey !== key)) {
      if (this.logTabs.some((t) => t.key === key)) {
        this.logUnread = { ...this.logUnread, [key]: true };
      }
    }
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

  /**
   * 打开某个任务的日志。
   *
   * 已有标签就切过去，没有就新开一个——启动新应用时也走这里「抢占」当前视图，
   * 旧标签不会丢，切回去还在。面板弹在独立窗口时也一样：主窗口只管改状态，
   * 改完广播出去，独立窗口自己会跟上来。
   */
  async openLog(projectId: string, taskId: string): Promise<void> {
    const key = taskKey(projectId, taskId);
    if (!this.logTabs.some((t) => t.key === key)) {
      this.logTabs = [...this.logTabs, { key, projectId, taskId }];
    }
    this.activateLog(key);

    // 面板在独立窗口里：新标签在那边出现，把那个窗口拉到前面来。
    // 用户刚点了「日志」却什么都看不到，比面板乱弹更难受。
    if (this.logDetached) void this.focusLogWindow();
  }

  /** 切到一个已打开的标签并展开抽屉，同时清掉它的未读标记。 */
  activateLog(key: string): void {
    this.logKey = key;
    this.logOpen = true;
    if (this.logUnread[key]) {
      const unread = { ...this.logUnread };
      delete unread[key];
      this.logUnread = unread;
    }

    const tab = this.logTabs.find((t) => t.key === key);
    if (tab) this.ensureLogContent(tab);
    this.syncLogState();
  }

  /** 收起抽屉。标签保留，下次展开还能回到这一条。 */
  closeLog(): void {
    this.logOpen = false;
  }

  /** 展开抽屉：回到上次看的那条日志，没有就取第一个标签。 */
  showLog(): void {
    // 面板在独立窗口里时，「展开」的意思是把它拉到前面，而不是在主窗口
    // 里再画一份——同一块面板不能同时出现在两个地方。
    if (this.logDetached) {
      void this.focusLogWindow();
      return;
    }

    if (this.logTabs.length === 0) return;
    const kept = this.logTabs.some((t) => t.key === this.logKey);
    this.activateLog(kept ? this.logKey : this.logTabs[0].key);
  }

  /** 抽屉开关，侧栏入口走这里。 */
  toggleLog(): void {
    if (this.logDetached) {
      void this.focusLogWindow();
      return;
    }

    if (this.logOpen) this.closeLog();
    else this.showLog();
  }

  /**
   * 关掉一个标签。
   *
   * 关掉的正是当前这条时，接管它右边那条（到头了就接管左边），用户连点几下
   * 关标签时视线不用来回跳；关到一条不剩就把抽屉收起来。
   */
  closeLogTab(key: string): void {
    const idx = this.logTabs.findIndex((t) => t.key === key);
    if (idx < 0) return;

    const rest = this.logTabs.filter((t) => t.key !== key);
    this.logTabs = rest;

    if (this.logUnread[key]) {
      const unread = { ...this.logUnread };
      delete unread[key];
      this.logUnread = unread;
    }

    if (rest.length === 0) {
      this.logKey = "";
      this.logOpen = false;
      this.syncLogState();

      // 独立窗口里关掉最后一个标签就没什么可看的了，跟着收回主窗口：
      // 留一个空窗口在那里，用户只能去猜怎么把面板拿回来。
      if (this.role === "log") void this.embedLog();
      return;
    }

    if (this.logKey === key) {
      this.activateLog(rest[Math.min(idx, rest.length - 1)].key);
    } else {
      this.syncLogState();
    }
  }

  /** 清空当前标签的日志内容。标签本身留着——用户要的是重新看一遍输出。 */
  clearLog(): void {
    const key = this.activeLogKey;
    if (!key) return;

    this.resetLog(key);
    // 另一个窗口也攒着同一份输出，只清自己这边会让两边看到的不一样。
    if (!isPreview) void Events.Emit(EVENT_LOG_CLEAR, { source: WINDOW_ID, key });
  }

  /**
   * 把整块日志面板弹成独立窗口。
   *
   * 弹出去之后面板只存在于独立窗口里：主窗口不再渲染它，之后启动新任务
   * 产生的日志也由独立窗口接管（状态走 bootpane:log-state 同步）。
   */
  async detachLog(): Promise<void> {
    if (this.logDetached) {
      await this.focusLogWindow();
      return;
    }
    if (!this.logKey) return;

    try {
      await api.detachLogWindow(JSON.stringify(this.logSnapshot()));
      this.logDetached = true;
      this.logOpen = true;
    } catch (e) {
      // 开不出窗口就保持内嵌：面板还在原地，用户看得见也还能用。
      this.notify(describeError(e), true);
    }
  }

  /**
   * 把面板交回主窗口（独立窗口上的「内嵌」）。
   *
   * 标签和当前标签早就通过 bootpane:log-state 同步过去了，这里只是通知
   * 主窗口「别再隐藏面板」，然后关掉自己。关窗时 Go 侧会再推一次关窗事件，
   * 主窗口那边是幂等的。
   */
  async embedLog(): Promise<void> {
    if (this.role !== "log") return;
    if (!isPreview) await Events.Emit(EVENT_LOG_EMBED, { source: WINDOW_ID });
    closeCurrentWindow();
  }

  /**
   * 从入口地址里读回面板状态（独立窗口专用）。
   *
   * 独立窗口是一个全新的前端实例，拿不到主窗口内存里的标签，所以弹窗时
   * 把状态编码进 URL。必须在第一次渲染之前调用，否则会先闪一帧空面板。
   */
  adoptLogQuery(search: string): void {
    const snap = parseLogSnapshot(new URLSearchParams(search).get(LOG_WINDOW_QUERY) ?? "");
    if (!snap) return;

    this.logTabs = snap.tabs;
    this.logKey = snap.key;
    this.logOpen = true;

    const active = this.logTabs.find((t) => t.key === this.logKey);
    if (active) this.ensureLogContent(active);
  }

  /** 把当前面板状态广播给另一个窗口。 */
  private syncLogState(): void {
    if (isPreview) return;
    void Events.Emit(EVENT_LOG_STATE, this.logSnapshot());
  }

  /** 面板状态快照：只有定位信息，日志正文由各窗口自己攒。 */
  private logSnapshot(): LogStatePayload {
    return { source: WINDOW_ID, tabs: this.logTabs, key: this.logKey };
  }

  /**
   * 采纳另一个窗口发来的面板状态。
   *
   * 整份替换而不是逐项合并：标签顺序、当前标签、有没有标签是同一份事实的
   * 不同侧面，分开合只会合出一个谁都没见过的中间状态。
   */
  private adoptLogState(p: LogStatePayload): void {
    if (!p || p.source === WINDOW_ID || !Array.isArray(p.tabs)) return;

    this.logTabs = p.tabs;
    this.logKey = p.key;

    // 新激活的那条可能是第一次见，自己手里没有它的输出。
    const active = this.logTabs.find((t) => t.key === this.logKey);
    if (active) this.ensureLogContent(active);

    if (this.logTabs.length === 0 && this.role === "log") void this.embedLog();
  }

  /** 独立窗口没了：把面板接回内嵌，当前标签和输出都还在。 */
  private embedLogPanel(): void {
    if (!this.logDetached) return;

    this.logDetached = false;
    // 独立窗口里可能已经把标签全关掉了，那就不要再拉起一个空面板。
    this.logOpen = this.logTabs.length > 0;
  }

  /**
   * 补一次历史输出。
   *
   * 后端环形缓冲里存着完整输出，但窗口刚开、刚重载、或者标签是另一个窗口
   * 开的，自己手里就没有内容——这时拉一次全量，之后靠 bootpane:log 增量。
   */
  private ensureLogContent(tab: LogTab): void {
    if (this.logs[tab.key]?.chars) return;

    void api
      .getTaskLog(tab.projectId, tab.taskId)
      .then((text) => {
        if (!text || this.logs[tab.key]?.chars) return;
        // 后端缓冲里同样是带 ANSI 的原始字节流，必须走同一套解析，
        // 否则「刚打开面板」和「跑了半天」看到的是两种格式。
        this.resetLog(tab.key);
        this.replaceLog(tab.key, parseAnsiChunk(this.ansiFor(tab.key), text));
      })
      .catch(() => {
        // 补不到就算了，后续事件来了照样能显示。
      });
  }

  /** 把独立窗口拉到前面；它已经不在（用户直接关了）就回落到内嵌面板。 */
  private async focusLogWindow(): Promise<void> {
    if (!this.logDetached) return;

    try {
      if (await api.focusLogWindow()) return;
    } catch {
      // 查不到窗口就等于已经关了，按内嵌处理。
    }

    this.logDetached = false;
    this.showLog();
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
      this.dropProjectLogs(id);
      await this.load();
      this.notify("已从列表移除（磁盘文件未受影响）");
    } catch (e) {
      this.notify(describeError(e), true);
    }
  }

  /**
   * 项目被移除后，收掉它开着的标签、日志和未读标记。
   *
   * 留着这些标签没有意义：任务已经查不到了，标签上只能显示 id，
   * 点开也只有一段再也不会增长的旧输出。
   */
  private dropProjectLogs(projectId: string): void {
    const dropped = this.logTabs.filter((t) => t.projectId === projectId);
    if (dropped.length === 0) return;

    for (const t of dropped) {
      this.resetLog(t.key);
      delete this.logUnread[t.key];
    }
    this.logTabs = this.logTabs.filter((t) => t.projectId !== projectId);
    if (!this.logTabs.some((t) => t.key === this.logKey)) {
      this.logKey = "";
      this.logOpen = false;
    }
    this.syncLogState();
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
   * 连着弹几个任务的标签只会让它来回跳，用户要的是「都起来了吗」，
   * 不是逐个看日志。
   */
  async startTask(projectId: string, taskId: string, autoLog = true): Promise<boolean> {
    const key = taskKey(projectId, taskId);
    // 重新启动意味着新一轮输出，旧的留着只会让人误判。
    this.resetLog(key);

    try {
      await api.startTask(projectId, taskId);
      await this.refreshRuntimes();

      // 新起来的应用抢占当前视图：就算正看着别人的日志也切过去。
      // 标签化之后这一步不再有代价——旧的输出留在它自己的标签里，
      // 想对照随时切回去，被切走不会丢任何东西。
      if (autoLog && this.settings.autoOpenLogOnStart) {
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
        // 重启也是一次「新实例启动」，和启动走同一条抢占规则。
        if (this.settings.autoOpenLogOnStart) void this.openLog(projectId, taskId);
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
