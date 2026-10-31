<script lang="ts">
  import type { Task } from "../../bindings/bootpane/internal/config/models";
  import { app } from "../lib/state.svelte";
  import { formatBytes, formatPercent } from "../lib/format";
  import { t } from "../lib/i18n/index.svelte";

  let {
    projectId,
    task,
    showProject = "",
  }: { projectId: string; task: Task; showProject?: string } = $props();

  const starred = $derived(app.isStarred(projectId, task.id));

  const rt = $derived(app.runtimeOf(projectId, task.id));
  const state = $derived(rt?.state ?? "idle");
  const busy = $derived(state === "running" || state === "starting");

  /** 有采样数据才显示资源徽章，避免启动瞬间闪一下 0%。 */
  const showUsage = $derived(busy && !!rt && rt.memoryBytes > 0);

  const portInfo = $derived(app.portOf(task));
  /**
   * 端口有人在监听，但 bootpane 自己没在跑这个任务。
   *
   * 这是最容易误导用户的场景：启动脚本发现「已在运行」就打印一行日志退出，
   * 进程退了但服务是活的。只报「已停止」等于说谎。
   */
  const foreignPort = $derived(!!portInfo?.listening && !busy);

  /** 端口占用者的说法，端口提示和重启确认里共用一套措辞。 */
  function whoOf(process: string | undefined, pid: number): string {
    return process ? t("task.who", { process, pid }) : t("task.pidOnly", { pid });
  }

  const portHint = $derived.by(() => {
    if (!portInfo?.listening) return "";
    return t("task.portHint", {
      port: portInfo.port,
      who: whoOf(portInfo.process, portInfo.pid),
    });
  });

  const restarting = $derived(!!app.restarting[`${projectId}/${task.id}`]);

  /** 这个任务对应的页面地址，空串表示不是网页类任务。 */
  const pageUrl = $derived(app.pageUrlOf(projectId, task.id));

  /**
   * 页面是不是该可点。
   *
   * 显式配了 openUrl 的随时可开（可能是已部署的域名）；只有按端口推导出来的
   * 地址才要求服务真的在跑——否则点了只会看到一个打不开的标签页。
   */
  const pageReady = $derived(
    !!pageUrl && (!!task.openUrl?.trim() || busy || !!portInfo?.listening),
  );

  /**
   * 打开页面。
   *
   * 没就绪时不禁用按钮：禁用状态在部分浏览器里连 tooltip 都不弹，用户只会
   * 看到一个灰掉的图标却不知道原因。改成点了给一句提示，至少能说清状况。
   */
  function openPage(): void {
    if (!pageReady) {
      app.notify(t("task.notReady", { label: task.label }));
      return;
    }
    void app.openTaskPage(projectId, task.id);
  }

  /**
   * 重启外部实例前必须二次确认。
   *
   * 那个进程可能是用户在另一个终端里手动起的，里面有未保存的状态——
   * 我们没有任何办法知道，只能把风险摆到明面上让他自己决定。
   */
  function confirmRestart(): void {
    const info = portInfo;
    if (!info?.listening) return;

    void app
      .confirm({
        title: t("task.restartTitle", { port: info.port }),
        body: t("task.restartBody", {
          who: whoOf(info.process, info.pid),
          label: task.label,
        }),
        detail: t("task.restartDetail"),
        confirmLabel: t("task.restartConfirm"),
        danger: true,
      })
      .then((ok) => {
        if (ok) void app.restartTask(projectId, task.id);
      });
  }

  const stateText = $derived.by(() => {
    switch (state) {
      case "starting":
        return rt?.ready ? t("task.stateStarting") : t("task.stateWaiting");
      case "running":
        return t("task.stateRunning");
      case "failed":
        return rt?.lastError
          ? t("task.stateFailedWith", { error: rt.lastError })
          : t("task.stateFailed");
      case "stopped":
        // 端口还在监听时，「已停止」是错的——服务还活着，只是不是这个进程。
        return foreignPort ? t("task.stateExited") : t("task.stateStopped");
      default:
        // 同理：从没启动过，但端口上有别人的实例。
        return foreignPort ? t("task.stateNotStarted") : t("task.stateIdle");
    }
  });

  const waitText = $derived.by(() => {
    const w = task.waitFor;
    if (!w) return "";
    if (w.type === "port") return t("task.waitPort", { port: w.port ?? 0 });
    if (w.type === "log") return t("task.waitLog", { log: w.log ?? "" });
    return "";
  });
</script>

<div class="task-card">
  <div class="task-top">
    {#if showProject}
      <span class="proj-badge" title={t("task.projectBadge", { name: showProject })}>{showProject}</span>
    {/if}
    <span class="task-label" title={task.label}>{task.label}</span>
    {#if pageUrl}
      <button
        class="page-btn"
        class:is-idle={!pageReady}
        title={t("task.openTitle", { url: pageUrl })}
        aria-label={t("task.openAria")}
        onclick={openPage}
      >
        <svg
          width="13"
          height="13"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M14 4h6v6" />
          <path d="M20 4l-9 9" />
          <path d="M18 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h5" />
        </svg>
      </button>
    {/if}
    <button
      class="star-btn"
      class:is-on={starred}
      title={starred ? t("task.unstar") : t("task.starHint")}
      aria-label={starred ? t("task.unstar") : t("task.star")}
      aria-pressed={starred}
      onclick={() => app.toggleStar(projectId, task.id)}
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill={starred ? "currentColor" : "none"}
        stroke="currentColor"
        stroke-width="1.7"
        stroke-linejoin="round"
      >
        <path d="M12 3.4l2.65 5.37 5.93.86-4.29 4.18 1.01 5.9L12 16.92l-5.3 2.79 1.01-5.9L3.42 9.63l5.93-.86z" />
      </svg>
    </button>
  </div>

  <div class="task-cmd" title={task.command}>{task.command}</div>

  {#if task.cwd || waitText || task.dependsOn?.length}
    <div class="task-meta">
      {#if task.cwd && task.cwd !== "."}
        <span>{t("task.dir")} <code>{task.cwd}</code></span>
      {/if}
      {#if waitText}
        <span>{waitText}</span>
      {/if}
      {#if task.dependsOn?.length}
        <span>{t("task.depends")} <code>{task.dependsOn.join(", ")}</code></span>
      {/if}
    </div>
  {/if}

  <div class="task-foot">
    <span class="state {state}" title={state === "failed" ? stateText : undefined}>
      <span class="dot"></span>
      <span class="state-text">{stateText}</span>
    </span>

    {#if foreignPort}
      <span class="port-flag" title={portHint}>{t("task.foreign")}</span>
    {:else if showUsage}
      <span
        class="usage"
        title={t("task.usageTitle")}
      >
        <span class="usage-cpu">CPU {formatPercent(rt?.cpuPercent ?? 0)}</span>
        <span class="usage-mem">{formatBytes(rt?.memoryBytes ?? 0)}</span>
      </span>
    {/if}

    <span class="foot-actions">
      <button
        class="btn btn-ghost btn-run"
        title={t("task.viewLog")}
        onclick={() => app.openLog(projectId, task.id)}
      >
        {t("task.log")}
      </button>

      {#if busy}
        <button class="btn btn-run" onclick={() => app.stopTask(projectId, task.id)}>
          {t("task.stop")}
        </button>
      {:else if foreignPort}
        <button class="btn btn-primary btn-run" disabled={restarting} onclick={confirmRestart}>
          {restarting ? t("task.restarting") : t("task.restart")}
        </button>
      {:else}
        <button class="btn btn-primary btn-run" onclick={() => app.startTask(projectId, task.id)}>
          {t("task.start")}
        </button>
      {/if}
    </span>
  </div>
</div>
