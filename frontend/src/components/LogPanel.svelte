<script lang="ts">
  import { tick } from "svelte";
  import { app } from "../lib/state.svelte";
  import { formatBytes, formatPercent } from "../lib/format";

  /**
   * 是不是跑在独立窗口里。
   *
   * 同一个面板两边共用：区别只在按钮（独立窗口只能收回内嵌，没有「收起」
   * 和「弹窗」）和外壳样式（独立窗口里它撑满整个窗口，不是底部抽屉）。
   */
  let { detached = false }: { detached?: boolean } = $props();

  let bodyEl: HTMLDivElement | undefined = $state();
  let tabsEl: HTMLDivElement | undefined = $state();
  /** 用户是否停留在底部。往上翻看历史时不能强行拽回去。 */
  let stickToBottom = $state(true);

  /*
   * 内嵌形态的抽屉高度：顶边有个拖拽把手，按住上下拖就改这里。
   * 独立窗口形态撑满窗口，不用这套。高度记进 localStorage，
   * 重启后沿用上次调的值；双击把手恢复默认。
   */
  const DRAWER_DEFAULT = 320;
  const DRAWER_MIN = 140;
  const DRAWER_STORE_KEY = "bootpane.logDrawerHeight";

  let drawerHeight = $state(loadDrawerHeight());

  function loadDrawerHeight(): number {
    const raw = Number(localStorage.getItem(DRAWER_STORE_KEY));
    if (Number.isFinite(raw) && raw >= DRAWER_MIN) return raw;
    return DRAWER_DEFAULT;
  }

  /** 拖拽进行中。模板里用它亮出把手提示线，所以得是响应式的。 */
  let resizing = $state(false);
  let resizeStartY = 0;
  let resizeStartHeight = 0;

  /** 拖不到天上去：至少给上面的内容留一截。 */
  function drawerMax(): number {
    return Math.max(DRAWER_MIN, window.innerHeight - 120);
  }

  function startResize(e: PointerEvent): void {
    resizing = true;
    resizeStartY = e.clientY;
    resizeStartHeight = drawerHeight;
    // capture 之后指针移出把手也照样收 move/up，不用再挂 window 监听。
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    document.body.classList.add("is-log-resizing");
  }

  function onResizeMove(e: PointerEvent): void {
    if (!resizing) return;
    // 把手在抽屉顶边：往上拖（dy 为正）抽屉变高。
    const dy = resizeStartY - e.clientY;
    drawerHeight = Math.min(Math.max(resizeStartHeight + dy, DRAWER_MIN), drawerMax());
  }

  function endResize(e: PointerEvent): void {
    if (!resizing) return;
    resizing = false;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    document.body.classList.remove("is-log-resizing");
    localStorage.setItem(DRAWER_STORE_KEY, String(Math.round(drawerHeight)));
  }

  function resetHeight(): void {
    drawerHeight = DRAWER_DEFAULT;
    localStorage.setItem(DRAWER_STORE_KEY, String(DRAWER_DEFAULT));
  }

  /** 标签条要显示的东西：任务名、所属项目、运行时状态、有没有新输出。 */
  const tabs = $derived(
    app.logTabs.map((tab) => {
      const project = app.projects.find((p) => p.id === tab.projectId);
      const task = (project?.tasks ?? []).find((t) => t.id === tab.taskId);
      return {
        key: tab.key,
        label: task?.label ?? tab.taskId,
        project: project?.name ?? "",
        title: project ? `${project.name} · ${task?.label ?? tab.taskId}` : (task?.label ?? tab.taskId),
        state: app.runtimeOf(tab.projectId, tab.taskId)?.state ?? "idle",
        unread: !!app.logUnread[tab.key],
      };
    }),
  );

  /**
   * 标签跨项目时才带项目名。
   *
   * 同一个项目里「启动网页端」「启动小程序端」本身就够区分，前面再挂一遍
   * 项目名纯属占地方；跨项目时才必须标明出处，否则同名任务分不清。
   */
  const multiProject = $derived(new Set(tabs.map((t) => t.project)).size > 1);

  const target = $derived.by(() => {
    const active = app.activeLog;
    if (!active) return null;
    const project = app.projects.find((p) => p.id === active.projectId);
    const task = (project?.tasks ?? []).find((t) => t.id === active.taskId);
    return {
      label: task?.label ?? active.taskId,
      command: task?.command ?? "",
      runtime: app.runtimeOf(active.projectId, active.taskId),
      portInfo: task ? app.portOf(task) : null,
    };
  });

  const runs = $derived(app.activeLogRuns);
  const runState = $derived(target?.runtime?.state ?? "idle");
  const live = $derived(runState === "running" || runState === "starting");
  /** 端口有人在听，但不是 bootpane 起的进程。 */
  const foreignPort = $derived(!!target?.portInfo?.listening && !live);

  const portHint = $derived.by(() => {
    const info = target?.portInfo;
    if (!info?.listening) return "";
    const who = info.process ? `${info.process}（PID ${info.pid}）` : `PID ${info.pid}`;
    return `端口 ${info.port} 正被 ${who} 占用，不是 BootPane 启动的。`;
  });

  // 日志变化后，只要用户还在底部就跟着滚到底。
  // 用 tick() 等 DOM 更新完再滚，否则拿到的还是旧的 scrollHeight。
  $effect(() => {
    if (!stickToBottom) return;
    if (runs.length === 0) return;
    void tick().then(() => {
      if (bodyEl) bodyEl.scrollTop = bodyEl.scrollHeight;
    });
  });

  // 切标签相当于换了一份输出：直接跳到新标签的底部。
  // 不能沿用上一个标签的滚动位置——从「翻着历史」切过去会落在新日志中段。
  $effect(() => {
    void app.activeLogKey;
    stickToBottom = true;
    void tick().then(() => {
      if (bodyEl) bodyEl.scrollTop = bodyEl.scrollHeight;
    });
  });

  // 新开的标签可能被挤在标签条右侧看不见的地方。
  // 切过去时把它滚进视野，否则点了「日志」会觉得什么都没发生。
  $effect(() => {
    void app.activeLogKey;
    tabsEl?.querySelector<HTMLElement>(".log-tab.is-active")?.scrollIntoView({ inline: "nearest", block: "nearest" });
  });

  function onScroll(): void {
    const el = bodyEl;
    if (!el) return;
    stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
  }
</script>

{#if target}
  <aside
    class="log-drawer"
    class:is-window={detached}
    class:is-resizing={resizing}
    style:height={detached ? undefined : `${drawerHeight}px`}
    aria-label="运行日志"
  >
    {#if !detached}
      <!-- 拖拽调高把手：绝对定位叠在抽屉顶边上，不占布局空间。
           双击恢复默认高度。 -->
      <div
        class="log-resize"
        class:is-active={resizing}
        role="separator"
        aria-orientation="horizontal"
        aria-label="拖拽调整日志面板高度"
        title="拖拽调整高度，双击恢复默认"
        onpointerdown={startResize}
        onpointermove={onResizeMove}
        onpointerup={endResize}
        onpointercancel={endResize}
        ondblclick={resetHeight}
      ></div>
    {/if}
    <div class="log-tabs-row">
      <div class="log-tabs" role="tablist" aria-label="日志标签" bind:this={tabsEl}>
        {#each tabs as tab (tab.key)}
          <div class="log-tab" class:is-active={tab.key === app.activeLogKey}>
            <button
              class="log-tab-main"
              role="tab"
              aria-selected={tab.key === app.activeLogKey}
              title={tab.title}
              onclick={() => app.activateLog(tab.key)}
            >
              <span class="dot {tab.state}"></span>
              {#if multiProject && tab.project}
                <span class="log-tab-proj">{tab.project}</span>
              {/if}
              <span class="log-tab-name">{tab.label}</span>
              {#if tab.unread}
                <span class="log-tab-new" title="有新输出"></span>
              {/if}
            </button>
            <button
              class="log-tab-close"
              aria-label={`关闭 ${tab.label} 的日志标签`}
              title="关闭"
              onclick={() => app.closeLogTab(tab.key)}
            >
              <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round">
                <line x1="5" y1="5" x2="19" y2="19" /><line x1="19" y1="5" x2="5" y2="19" />
              </svg>
            </button>
          </div>
        {/each}
      </div>

      <div class="log-tab-actions">
        {#if detached}
          <button
            class="log-tab-btn"
            title="把日志面板收回主窗口"
            aria-label="内嵌回主窗口"
            onclick={() => app.embedLog()}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 3v10" /><path d="m8 9 4 4 4-4" /><path d="M4 19h16" />
            </svg>
          </button>
        {:else}
          <button
            class="log-tab-btn"
            title="把整个日志面板弹到独立窗口"
            aria-label="弹窗"
            onclick={() => app.detachLog()}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <path d="M14 4h6v6" /><path d="M20 4 12 12" /><path d="M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4" />
            </svg>
          </button>
          <button
            class="log-tab-btn"
            title="收起日志面板"
            aria-label="收起"
            onclick={() => app.closeLog()}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <path d="M5 6h14" /><path d="m7 11 5 5 5-5" />
            </svg>
          </button>
        {/if}
      </div>
    </div>

    <header class="log-head">
      <span class="state {runState}">
        <span class="dot"></span>
        {#if runState === "running"}运行中{:else if runState === "starting"}启动中{:else if runState === "failed"}已失败{:else if runState === "stopped"}{foreignPort ? "已退出" : "已停止"}{:else}{foreignPort ? "未启动" : "未运行"}{/if}
      </span>

      <span class="log-title" title={target.command}>{target.label}</span>

      {#if foreignPort && target.portInfo}
        <span class="port-flag" title={portHint}>
          {target.portInfo.process || "外部进程"} 占用 {target.portInfo.port}
        </span>
      {:else if live && target.runtime && target.runtime.memoryBytes > 0}
        <span class="usage" title="整棵进程树合计">
          <span class="usage-cpu">CPU {formatPercent(target.runtime.cpuPercent)}</span>
          <span class="usage-mem">{formatBytes(target.runtime.memoryBytes)}</span>
          <span class="usage-proc">{target.runtime.processes} 进程</span>
        </span>
      {/if}

      <span class="foot-gap"></span>

      <button class="btn btn-ghost btn-mini" onclick={() => app.clearLog()}>清空</button>
    </header>

    <div class="log-body" bind:this={bodyEl} onscroll={onScroll}>
      {#if runs.length > 0}
        <pre>{#each runs as run}{#if run.cls || run.style}<span class={run.cls} style={run.style}>{run.text}</span>{:else}{run.text}{/if}{/each}</pre>
      {:else}
        <div class="log-empty">
          {live ? "等待输出…" : "还没有日志。点启动按钮后这里会显示实时输出。"}
        </div>
      {/if}
    </div>
  </aside>
{/if}
