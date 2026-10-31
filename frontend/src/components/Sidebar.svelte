<script lang="ts">
  import { Dialogs } from "@wailsio/runtime";
  import { app, STARRED_VIEW } from "../lib/state.svelte";
  import { t } from "../lib/i18n/index.svelte";
  import type { ResolvedProject } from "../../bindings/bootpane/internal/config/models";

  let query = $state("");

  /** 星标视图里涉及的、当前还在的项目数。 */
  const starredProjectCount = $derived(
    new Set(app.starredTasks.map(({ project }) => project.id)).size,
  );

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return app.projects;
    return app.projects.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.path.toLowerCase().includes(q) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(q)),
    );
  });

  /** 列表上的小圆点：目录丢失 > 配置错误 > 已配置任务 > 空。 */
  function dotClass(p: ResolvedProject): string {
    if (!p.exists) return "gone";
    if ((p.issues ?? []).some((i) => i.level === "error")) return "err";
    if ((p.tasks ?? []).length > 0) return "ok";
    return "";
  }

  function subtitle(p: ResolvedProject): string {
    if (!p.exists) return t("sidebar.dirMissing");
    if (p.configError) return t("sidebar.configError");
    const count = (p.tasks ?? []).length;
    if (count === 0) return t("sidebar.noTasks");
    return t("sidebar.taskCount", { count });
  }

  async function pickFolder(): Promise<void> {
    try {
      const picked = await Dialogs.OpenFile({
        Title: t("sidebar.pickTitle"),
        CanChooseDirectories: true,
        CanChooseFiles: false,
        AllowsMultipleSelection: false,
        ButtonText: t("sidebar.pickAdd"),
      });
      const path = Array.isArray(picked) ? picked[0] : picked;
      if (path) await app.addProject(path);
    } catch (e) {
      app.notify(t("sidebar.pickFailed", { error: String(e) }), true);
    }
  }
</script>

<aside class="rail">
  <div class="rail-head">
    <input class="search" placeholder={t("sidebar.search")} bind:value={query} spellcheck="false" />
    <button class="icon-btn" title={t("sidebar.add")} aria-label={t("sidebar.add")} onclick={pickFolder}>
      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
      </svg>
    </button>
  </div>

  <div class="rail-list">
    <button
      class="proj-item starred-item"
      class:is-active={app.showStarred}
      onclick={() => app.select(STARRED_VIEW)}
      title={t("sidebar.starredTitle")}
    >
      <div class="proj-row">
        <svg
          class="star-glyph"
          width="13"
          height="13"
          viewBox="0 0 24 24"
          fill={app.starredTasks.length > 0 ? "currentColor" : "none"}
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linejoin="round"
        >
          <path d="M12 3.4l2.65 5.37 5.93.86-4.29 4.18 1.01 5.9L12 16.92l-5.3 2.79 1.01-5.9L3.42 9.63l5.93-.86z" />
        </svg>
        <span class="proj-name">{t("sidebar.starred")}</span>
        {#if app.starredTasks.length > 0}
          <span class="count-chip">{app.starredTasks.length}</span>
        {/if}
      </div>
      <div class="proj-sub">
        {app.starredTasks.length > 0
          ? t("sidebar.starredFrom", { count: starredProjectCount })
          : t("starred.emptyTitle")}
      </div>
    </button>

    <div class="rail-sep"></div>

    {#each filtered as p (p.id)}
      <button
        class="proj-item"
        class:is-active={p.id === app.selectedId}
        onclick={() => app.select(p.id)}
        title={p.path}
      >
        <div class="proj-row">
          <span class="dot {dotClass(p)}"></span>
          <span class="proj-name">{p.name}</span>
        </div>
        <div class="proj-sub">{subtitle(p)}</div>
      </button>
    {:else}
      <div class="proj-sub" style="padding: 12px 10px;">
        {app.loading
          ? t("sidebar.loading")
          : query
            ? t("sidebar.noMatch")
            : t("app.emptyNoProjectsTitle")}
      </div>
    {/each}
  </div>

  <div class="rail-foot">
    <span>{t("sidebar.projectCount", { count: app.projects.length })}</span>
    <span class="foot-links">
      {#if app.logTabs.length > 0}
        <button
          class="link-btn"
          class:is-on={app.logOpen}
          title={t("sidebar.logToggle")}
          onclick={() => app.toggleLog()}
        >
          {t("sidebar.log", { count: app.logTabs.length })}
        </button>
      {/if}
      <button
        class="link-icon"
        class:is-on={app.showSettings}
        title={t("sidebar.settings")}
        aria-label={t("sidebar.settings")}
        onclick={() => app.openSettings()}
      >
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.7"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <circle cx="12" cy="12" r="3.1" />
          <path
            d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1.08-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
          />
        </svg>
      </button>
    </span>
  </div>
</aside>
