<script lang="ts">
  import type { Task } from "../../bindings/bootpane/internal/config/models";
  import { app } from "../lib/state.svelte";

  let { projectId, task }: { projectId: string; task: Task } = $props();

  const rt = $derived(app.runtimeOf(projectId, task.id));
  const state = $derived(rt?.state ?? "idle");
  const busy = $derived(state === "running" || state === "starting");

  const stateText = $derived.by(() => {
    switch (state) {
      case "starting":
        return rt?.ready ? "启动中" : "等待就绪";
      case "running":
        return "运行中";
      case "failed":
        return rt?.lastError ? `失败：${rt.lastError}` : "启动失败";
      case "stopped":
        return "已停止";
      default:
        return "未运行";
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
    {#if task.origin === "local"}
      <span class="badge local" title="来自全局注册表的个人配置，不会提交进仓库">本地</span>
    {/if}
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
    {#if busy}
      <button class="btn btn-run" onclick={() => app.stopTask(projectId, task.id)}>停止</button>
    {:else}
      <button class="btn btn-primary btn-run" onclick={() => app.startTask(projectId, task.id)}>
        启动
      </button>
    {/if}
  </div>
</div>
