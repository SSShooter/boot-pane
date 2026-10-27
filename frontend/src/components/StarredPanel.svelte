<script lang="ts">
  import TaskCard from "./TaskCard.svelte";
  import { app } from "../lib/state.svelte";

  /**
   * 跨项目聚合视图。
   *
   * 这里刻意不按项目分组：星标的用途就是「把散在各个项目里的几个按钮
   * 拉到一起统一操作」，再套一层分组反而把距离拉远了。项目名做成卡片
   * 上的小徽章，需要区分时一眼能看到，不需要时不占视线。
   */
  const total = $derived(app.starredTasks.length);
  const projectCount = $derived(new Set(app.starredTasks.map(({ project }) => project.id)).size);
  const runningCount = $derived(
    app.starredTasks.filter(({ project, task }) => app.isBusy(project.id, task.id)).length,
  );
</script>

<div class="panel">
  <div class="panel-head">
    <div class="panel-title">
      <div class="title-row">
        <h1>星标任务</h1>
        {#if total > 0}
          <span class="count-chip">{runningCount} / {total} 运行中</span>
        {/if}
      </div>
      <div class="panel-path">
        {#if total > 0}
          来自 {projectCount} 个项目的 {total} 个任务
        {:else}
          还没有星标任务
        {/if}
      </div>
    </div>
    <div class="head-actions">
      <button class="btn btn-primary" disabled={total === 0} onclick={() => app.startStarred()}>
        全部启动
      </button>
      <button class="btn" disabled={runningCount === 0} onclick={() => app.stopStarred()}>
        全部停止
      </button>
    </div>
  </div>

  {#if total === 0}
    <div class="empty" style="height: auto; padding: 44px 0;">
      <div class="empty-icon">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round">
          <path d="M12 3.4l2.65 5.37 5.93.86-4.29 4.18 1.01 5.9L12 16.92l-5.3 2.79 1.01-5.9L3.42 9.63l5.93-.86z" />
        </svg>
      </div>
      <h2>还没有星标任务</h2>
      <p>
        在任意项目的任务卡片右上角点一下星形图标，任务就会聚合到这里。
        跨项目的常用按钮放在一起，可以一次性全部启动或全部停止。
      </p>
    </div>
  {:else}
    <div class="task-grid starred-grid">
      {#each app.starredTasks as { project, task } (`${project.id}/${task.id}`)}
        <TaskCard projectId={project.id} {task} showProject={project.name} />
      {/each}
    </div>
  {/if}
</div>
