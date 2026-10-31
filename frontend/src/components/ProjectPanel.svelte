<script lang="ts">
  import type { ResolvedProject, Task } from "../../bindings/bootpane/internal/config/models";
  import TaskCard from "./TaskCard.svelte";
  import { app } from "../lib/state.svelte";
  import { t } from "../lib/i18n/index.svelte";

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
        title: t("project.removeTitle", { name: project.name }),
        body: t("project.removeBody"),
        detail: t("project.removeDetail"),
        confirmLabel: t("project.remove"),
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
        <div class="seg" role="group" aria-label={t("project.sourceAria")}>
          <button
            class:is-on={!project.useRepoConfig}
            title={t("project.globalHint")}
            onclick={() => app.setUseRepoConfig(project.id, false)}
          >
            {t("project.global")}
          </button>
          <button
            class:is-on={project.useRepoConfig}
            title={t("project.repoHint")}
            onclick={() => app.setUseRepoConfig(project.id, true)}
          >
            {t("project.repo")}
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
        {t("project.generate")}
      </button>
      <button class="btn" onclick={() => app.openEditor(project.id)}>{t("project.edit")}</button>
      <button
        class="btn"
        title={app.openWith
          ? t("project.openWithTitle", { name: app.openWithLabel, path: project.path })
          : t("project.openDirTitle")}
        onclick={() => app.openProjectDir(project.id)}
      >
        {app.openWith ? t("project.openWith", { name: app.openWithLabel }) : t("project.openDir")}
      </button>
      <button class="btn btn-danger" onclick={confirmRemove}>{t("project.remove")}</button>
    </div>
  </div>

  <div class="panel-body">
    {#if errors.length > 0}
      <div class="banner err">
        <div>
          <strong>{t("project.errorsTitle")}</strong>
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
          <strong>{t("project.warnsTitle")}</strong>
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
          {t("project.repoBannerA")} <code>.bootpane.json</code>{t("project.repoBannerB")}
          <button class="link-btn" onclick={() => app.setUseRepoConfig(project.id, true)}>
            {t("project.repoSwitch")}
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
        <h2>{t("project.noTasksTitle")}</h2>
        <p>
          {t("project.noTasksBody")}
        </p>
        <div class="empty-actions">
          <button class="btn btn-primary" onclick={() => app.openGenerator(project.id)}>
            {t("project.generate")}
          </button>
          <button class="btn" onclick={() => app.openEditor(project.id)}>{t("project.writeJson")}</button>
          <button class="btn" onclick={() => app.openConfigDir()}>{t("settings.openConfigDir")}</button>
        </div>
      </div>
    {:else}
      {#each groups as group (group.name)}
        <section class="group">
          <div class="group-head">
            <span class="group-name">{group.name || t("project.defaultGroup")}</span>
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
