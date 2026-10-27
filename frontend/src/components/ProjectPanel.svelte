<script lang="ts">
  import type { ResolvedProject } from "../../bindings/bootpane/internal/config/models";
  import TaskCard from "./TaskCard.svelte";
  import { app } from "../lib/state.svelte";

  let { project }: { project: ResolvedProject } = $props();

  const groups = $derived.by(() => {
    const map = new Map<string, ResolvedProject["tasks"]>();
    for (const t of project.tasks) {
      const key = t.group?.trim() ?? "";
      const bucket = map.get(key);
      if (bucket) bucket.push(t);
      else map.set(key, [t]);
    }
    return [...map.entries()].map(([name, tasks]) => ({ name, tasks }));
  });

  const errors = $derived(project.issues.filter((i) => i.level === "error"));
  const warns = $derived(project.issues.filter((i) => i.level === "warn"));

  function confirmRemove(): void {
    const ok = confirm(`从列表移除「${project.name}」？\n磁盘上的仓库文件不会被删除。`);
    if (ok) app.removeProject(project.id);
  }

  const snippet = `{
  "version": 1,
  "tasks": [
    {
      "id": "web",
      "label": "启动前端",
      "group": "前端",
      "command": "pnpm dev",
      "waitFor": { "type": "port", "port": 5173 }
    }
  ]
}`;
</script>

<div class="panel">
  <div class="panel-head">
    <div class="panel-title">
      <h1>{project.name}</h1>
      <div class="panel-path">{project.path}</div>
      {#if project.tags.length > 0}
        <div class="tag-row">
          {#each project.tags as tag}
            <span class="tag">{tag}</span>
          {/each}
        </div>
      {/if}
    </div>
    <div class="head-actions">
      <button class="btn" onclick={() => app.openProjectDir(project.id)}>打开目录</button>
      <button class="btn btn-danger" onclick={confirmRemove}>移除</button>
    </div>
  </div>

  {#if errors.length > 0}
    <div class="banner err">
      <div>
        <strong>配置有问题，启动前需要修正</strong>
        <ul class="banner-list">
          {#each errors as issue}
            <li>{issue.taskId ? `[${issue.taskId}] ` : ""}{issue.message}</li>
          {/each}
        </ul>
      </div>
    </div>
  {/if}

  {#if warns.length > 0}
    <div class="banner warn">
      <div>
        <strong>提示</strong>
        <ul class="banner-list">
          {#each warns as issue}
            <li>{issue.taskId ? `[${issue.taskId}] ` : ""}{issue.message}</li>
          {/each}
        </ul>
      </div>
    </div>
  {/if}

  {#if project.exists && project.tasks.length === 0}
    <div class="empty" style="height: auto; padding: 44px 0;">
      <div class="empty-icon">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round">
          <rect x="3" y="4" width="18" height="16" rx="2" /><line x1="3" y1="10" x2="21" y2="10" />
        </svg>
      </div>
      <h2>这个项目还没有启动按钮</h2>
      <p>
        在仓库根目录新建 <code>.bootpane.json</code> 即可定义。一个项目可以配置任意多个启动项，
        用 <code>group</code> 分组、<code>dependsOn</code> 控制先后顺序。
      </p>
      <pre class="snippet">{snippet}</pre>
    </div>
  {:else}
    {#each groups as group (group.name)}
      <section class="group">
        <div class="group-head">
          <span class="group-name">{group.name || "启动项"}</span>
          <span class="group-rule"></span>
        </div>
        <div class="task-grid">
          {#each group.tasks as task (task.id)}
            <TaskCard projectId={project.id} {task} />
          {/each}
        </div>
      </section>
    {/each}
  {/if}
</div>
