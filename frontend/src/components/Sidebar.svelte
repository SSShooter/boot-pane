<script lang="ts">
  import { Dialogs } from "@wailsio/runtime";
  import { app } from "../lib/state.svelte";
  import type { ResolvedProject } from "../../bindings/bootpane/internal/config/models";

  let query = $state("");

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return app.projects;
    return app.projects.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.path.toLowerCase().includes(q) ||
        p.tags.some((t) => t.toLowerCase().includes(q)),
    );
  });

  /** 列表上的小圆点：目录丢失 > 配置错误 > 已配置任务 > 空。 */
  function dotClass(p: ResolvedProject): string {
    if (!p.exists) return "gone";
    if (p.issues.some((i) => i.level === "error")) return "err";
    if (p.tasks.length > 0) return "ok";
    return "";
  }

  function subtitle(p: ResolvedProject): string {
    if (!p.exists) return "目录不存在";
    if (p.configError) return "配置有误";
    if (p.tasks.length === 0) return "未配置启动项";
    return `${p.tasks.length} 个启动项`;
  }

  async function pickFolder(): Promise<void> {
    try {
      const picked = await Dialogs.OpenFile({
        Title: "选择项目目录",
        CanChooseDirectories: true,
        CanChooseFiles: false,
        AllowsMultipleSelection: false,
        ButtonText: "添加",
      });
      const path = Array.isArray(picked) ? picked[0] : picked;
      if (path) await app.addProject(path);
    } catch (e) {
      app.notify("打开目录选择器失败：" + String(e), true);
    }
  }
</script>

<aside class="rail">
  <div class="rail-head">
    <div class="brand"><span class="brand-mark">B</span> bootpane</div>
    <button class="icon-btn" title="添加项目" aria-label="添加项目" onclick={pickFolder}>
      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
      </svg>
    </button>
  </div>

  <div class="search-wrap">
    <input class="search" placeholder="搜索项目 / 标签" bind:value={query} spellcheck="false" />
  </div>

  <div class="rail-list">
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
        {app.loading ? "正在加载…" : query ? "没有匹配的项目" : "还没有添加项目"}
      </div>
    {/each}
  </div>

  <div class="rail-foot">
    <span>{app.projects.length} 个项目</span>
    <button class="link-btn" onclick={() => app.openConfigDir()}>配置目录</button>
  </div>
</aside>
