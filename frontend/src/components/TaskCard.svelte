<script lang="ts">
  import type { Task } from "../../bindings/bootpane/internal/config/models";
  import { app } from "../lib/state.svelte";
  import { formatBytes, formatPercent } from "../lib/format";

  let { projectId, task }: { projectId: string; task: Task } = $props();

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

  const portHint = $derived.by(() => {
    if (!portInfo?.listening) return "";
    const who = portInfo.process
      ? `${portInfo.process}（PID ${portInfo.pid}）`
      : `PID ${portInfo.pid}`;
    return `端口 ${portInfo.port} 正被 ${who} 占用，不是 bootpane 启动的。点「启动」时脚本可能会自己判断并跳过。`;
  });

  const stateText = $derived.by(() => {
    switch (state) {
      case "starting":
        return rt?.ready ? "启动中" : "等待就绪";
      case "running":
        return "运行中";
      case "failed":
        return rt?.lastError ? `失败：${rt.lastError}` : "启动失败";
      case "stopped":
        // 端口还在监听时，「已停止」是错的——服务还活着，只是不是这个进程。
        return foreignPort ? "已退出" : "已停止";
      default:
        // 同理：从没启动过，但端口上有别人的实例。
        return foreignPort ? "未启动" : "未运行";
    }
  });

  const waitText = $derived.by(() => {
    const w = task.waitFor;
    if (!w) return "";
    if (w.type === "port") return `等端口 ${w.port}`;
    if (w.type === "log") return `等日志 “${w.log}”`;
    return "";
  });
</script>

<div class="task-card">
  <div class="task-top">
    <span class="task-label" title={task.label}>{task.label}</span>
  </div>

  <div class="task-cmd" title={task.command}>{task.command}</div>

  {#if task.cwd || waitText || task.dependsOn?.length}
    <div class="task-meta">
      {#if task.cwd && task.cwd !== "."}
        <span>目录 <code>{task.cwd}</code></span>
      {/if}
      {#if waitText}
        <span>{waitText}</span>
      {/if}
      {#if task.dependsOn?.length}
        <span>依赖 <code>{task.dependsOn.join(", ")}</code></span>
      {/if}
    </div>
  {/if}

  <div class="task-foot">
    <span class="state {state}">
      <span class="dot"></span>
      {stateText}
    </span>

    {#if foreignPort}
      <span class="port-flag" title={portHint}>外部运行中</span>
    {:else if showUsage}
      <span
        class="usage"
        title="整棵进程树合计（含 pnpm → node → vite 等子进程）"
      >
        <span class="usage-cpu">CPU {formatPercent(rt?.cpuPercent ?? 0)}</span>
        <span class="usage-mem">{formatBytes(rt?.memoryBytes ?? 0)}</span>
      </span>
    {/if}

    <span class="foot-gap"></span>

    <button
      class="btn btn-ghost btn-run"
      title="查看运行日志"
      onclick={() => app.openLog(projectId, task.id)}
    >
      日志
    </button>

    {#if busy}
      <button class="btn btn-run" onclick={() => app.stopTask(projectId, task.id)}>停止</button>
    {:else}
      <button class="btn btn-primary btn-run" onclick={() => app.startTask(projectId, task.id)}>
        启动
      </button>
    {/if}
  </div>
</div>
