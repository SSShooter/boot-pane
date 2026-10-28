<script lang="ts">
  import { tick } from "svelte";
  import { app } from "../lib/state.svelte";
  import { formatBytes, formatPercent } from "../lib/format";

  let bodyEl: HTMLDivElement | undefined = $state();
  /** 用户是否停留在底部。往上翻看历史时不能强行拽回去。 */
  let stickToBottom = $state(true);

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
    return `端口 ${info.port} 正被 ${who} 占用，不是 bootpane 启动的。`;
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

  function onScroll(): void {
    const el = bodyEl;
    if (!el) return;
    stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
  }
</script>

{#if target}
  <aside class="log-drawer" aria-label="运行日志">
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
      <button class="btn btn-ghost btn-mini" onclick={() => app.closeLog()}>收起</button>
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
