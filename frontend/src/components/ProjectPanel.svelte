<script lang="ts">
  import type { ResolvedProject, Task } from "../../bindings/bootpane/internal/config/models";
  import TaskCard from "./TaskCard.svelte";
  import { app } from "../lib/state.svelte";

  let { project }: { project: ResolvedProject } = $props();

  // 后端返回的是 Go 的 nil slice，反序列化后可能是 null；组件里到处都要遍历
  // 这两份列表，统一在这里折成空数组，避免每个用到的地方各写一次判空。
  const tasks = $derived(project.tasks ?? []);
  const tags = $derived(project.tags ?? []);

  const groups = $derived.by(() => {
    const map = new Map<string, Task[]>();
    for (const t of tasks) {
      const key = t.group?.trim() ?? "";
      const bucket = map.get(key);
      if (bucket) bucket.push(t);
      else map.set(key, [t]);
    }
    return [...map.entries()].map(([name, tasks]) => ({ name, tasks }));
  });

  const errors = $derived((project.issues ?? []).filter((i) => i.level === "error"));
  const warns = $derived((project.issues ?? []).filter((i) => i.level === "warn"));

  /** 仓库里自带配置，但当前用的是全局配置——提醒用户可以切过去。 */
  const repoHint = $derived(project.exists && project.hasRepoConfig && !project.useRepoConfig);

  function confirmRemove(): void {
    void app
      .confirm({
        title: `从列表移除「${project.name}」？`,
        body: "这个项目会从 bootpane 的项目列表里消失，随时可以通过目录重新添加。",
        detail: "磁盘上的仓库文件不会被删除。",
        confirmLabel: "移除",
        danger: true,
      })
      .then((ok) => {
        if (ok) void app.removeProject(project.id);
      });
  }
</script>

<div class="panel">
  <div class="panel-head">
    <div class="panel-title">
      <div class="title-row">
        <!-- 窄窗口下长项目名会被省略号截断，鼠标悬停能看到全名 -->
        <h1 title={project.name}>{project.name}</h1>
        <div class="seg" role="group" aria-label="配置来源">
          <button
            class:is-on={!project.useRepoConfig}
            title="使用本机注册表里的配置"
            onclick={() => app.setUseRepoConfig(project.id, false)}
          >
            全局
          </button>
          <button
            class:is-on={project.useRepoConfig}
            title="使用仓库根目录的 .bootpane.json"
            onclick={() => app.setUseRepoConfig(project.id, true)}
          >
            项目内
          </button>
        </div>
      </div>
      {#if project.description}
        <div class="panel-desc">{project.description}</div>
      {/if}
      <div class="panel-path">{project.path}</div>
      {#if tags.length > 0}
        <div class="tag-row">
          {#each tags as tag}
            <span class="tag">{tag}</span>
          {/each}
        </div>
      {/if}
    </div>
    <div class="head-actions">
      <button class="btn btn-primary" onclick={() => app.openGenerator(project.id)}>
        生成配置
      </button>
      <button class="btn" onclick={() => app.openEditor(project.id)}>编辑配置</button>
      <button
        class="btn"
        title={app.openWith ? `用 ${app.openWithLabel} 打开 ${project.path}` : "在系统文件管理器里打开"}
        onclick={() => app.openProjectDir(project.id)}
      >
        {app.openWith ? `用 ${app.openWithLabel} 打开` : "打开目录"}
      </button>
      <button class="btn btn-danger" onclick={confirmRemove}>移除</button>
    </div>
  </div>

  <div class="panel-body">
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

    {#if repoHint && errors.length === 0}
      <div class="banner info">
        <div>
          这个仓库自带 <code>.bootpane.json</code>，当前用的是全局配置。
          <button class="link-btn" onclick={() => app.setUseRepoConfig(project.id, true)}>
            切换过去
          </button>
        </div>
      </div>
    {/if}

    {#if project.exists && tasks.length === 0}
      <div class="empty" style="height: auto; padding: 44px 0;">
        <div class="empty-icon">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round">
            <rect x="3" y="4" width="18" height="16" rx="2" /><line x1="3" y1="10" x2="21" y2="10" />
          </svg>
        </div>
        <h2>这个项目还没有启动按钮</h2>
        <p>
          让 AI 读一遍仓库结构，自动生成一份配置——复制 prompt 到项目目录下用，
          再把结果粘回来就行。也可以直接手写 JSON。
        </p>
        <div class="empty-actions">
          <button class="btn btn-primary" onclick={() => app.openGenerator(project.id)}>
            生成配置
          </button>
          <button class="btn" onclick={() => app.openEditor(project.id)}>手写 JSON</button>
          <button class="btn" onclick={() => app.openConfigDir()}>打开配置目录</button>
        </div>
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
</div>
