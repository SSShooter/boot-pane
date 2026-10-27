<script lang="ts">
  import type {
    Issue,
    ResolvedProject,
  } from "../../bindings/bootpane/internal/config/models";
  import { api, isPreview } from "../lib/api";
  import { app } from "../lib/state.svelte";

  let { project, onclose }: { project: ResolvedProject; onclose: () => void } = $props();

  let text = $state("");
  /** 打开时的原文，用来判断有没有未保存的改动。 */
  let original = $state("");
  let loading = $state(true);
  let saving = $state(false);
  let loadError = $state("");
  let errorMsg = $state("");
  let issues = $state<Issue[]>([]);

  const dirty = $derived(text !== original);

  /**
   * 写入目标。
   *
   * 必须写清楚——项目内配置模式下保存会直接改用户工作区里的文件，
   * 这件事不能在界面上含糊过去。
   */
  const target = $derived.by(() => {
    if (!project.useRepoConfig) return { label: "全局注册表", path: "workspace.json" };
    return { label: "仓库内配置", path: `${project.path}\\.bootpane.json` };
  });

  /**
   * 前端只做语法预检。
   *
   * 真正的校验（工作目录存不存在、依赖指向的任务在不在）全在后端，
   * 这里提前拦住明显写坏的 JSON，省得白跑一趟还能给出更快的反馈。
   */
  const syntaxError = $derived.by(() => {
    if (!text.trim()) return "内容为空";
    try {
      JSON.parse(text);
      return "";
    } catch (e) {
      return e instanceof Error ? e.message : String(e);
    }
  });

  $effect(() => {
    const id = project.id;
    loading = true;
    loadError = "";
    api
      .configJson(id)
      .then((raw) => {
        text = raw;
        original = raw;
      })
      .catch((e) => {
        loadError = describe(e);
      })
      .finally(() => {
        loading = false;
      });
  });

  function describe(e: unknown): string {
    if (typeof e === "string") return e;
    if (e instanceof Error) return e.message;
    return String(e);
  }

  /** 重新排版。语法错的时候不动内容，免得把用户写了一半的东西弄乱。 */
  function format(): void {
    try {
      text = JSON.stringify(JSON.parse(text), null, 2) + "\n";
    } catch {
      // 语法错误时按钮本来就是禁用的，这里只是兜底。
    }
  }

  function requestClose(): void {
    if (saving) return;
    if (dirty && !confirm("有未保存的修改，确定关闭？")) return;
    onclose();
  }

  async function save(): Promise<void> {
    if (syntaxError || saving) return;

    saving = true;
    errorMsg = "";
    issues = [];
    try {
      const res = await api.saveConfigJson(project.id, text);
      issues = res.issues ?? [];
      if (!res.ok) {
        errorMsg = res.error || "保存失败";
        return;
      }
      await app.load();
      // 保存成功也可能带回 warn（比如端口重复），这些会在主界面的横幅里显示，
      // 这里只把条数点出来，免得用户以为「没报错就是全对」。
      app.notify(
        issues.length > 0
          ? `已保存 ${res.imported} 个启动项，另有 ${issues.length} 条提示`
          : `已保存 ${res.imported} 个启动项`,
      );
      original = text;
      onclose();
    } catch (e) {
      errorMsg = describe(e);
    } finally {
      saving = false;
    }
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === "Escape") requestClose();
    // Ctrl/Cmd + S 直接保存，编辑大段 JSON 时手不用离开键盘。
    if ((e.ctrlKey || e.metaKey) && e.key === "s") {
      e.preventDefault();
      void save();
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="overlay">
  <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="editor-title">
    <header class="dialog-head">
      <div>
        <h2 id="editor-title">编辑配置</h2>
        <p class="dialog-sub">
          保存到{target.label} <code class="path-chip">{target.path}</code>
        </p>
      </div>
      <button class="icon-btn" aria-label="关闭" onclick={requestClose}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="6" y1="6" x2="18" y2="18" /><line x1="18" y1="6" x2="6" y2="18" />
        </svg>
      </button>
    </header>

    <div class="dialog-body editor-body">
      {#if loading}
        <div class="prompt-loading">正在读取配置…</div>
      {:else if loadError}
        <div class="dialog-error">{loadError}</div>
      {:else}
        <div class="editor-bar">
          <button class="btn btn-mini" disabled={!!syntaxError} onclick={format}>格式化</button>
          {#if syntaxError}
            <span class="syntax-flag err">JSON 语法有误</span>
          {:else}
            <span class="syntax-flag ok">JSON 语法正常</span>
          {/if}
          <span class="editor-hint">Ctrl+S 保存</span>
        </div>

        <textarea
          class="json-editor"
          bind:value={text}
          spellcheck="false"
          autocomplete="off"
          autocapitalize="off"
          wrap="off"
        ></textarea>

        {#if syntaxError}
          <div class="dialog-error">JSON 语法有误：{syntaxError}</div>
        {/if}
        {#if errorMsg}
          <div class="dialog-error">{errorMsg}</div>
        {/if}
        {#if issues.length > 0}
          <ul class="dialog-issues">
            {#each issues as issue}
              <li class:err={issue.level === "error"}>
                {issue.taskId ? `[${issue.taskId}] ` : ""}{issue.message}
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <footer class="dialog-foot">
      {#if isPreview}
        <span class="dialog-note">浏览器预览模式：改动只存在内存里</span>
      {:else if project.useRepoConfig}
        <span class="dialog-note">保存会直接修改仓库里的文件，记得提交</span>
      {/if}
      <button class="btn" onclick={requestClose}>取消</button>
      <button
        class="btn btn-primary"
        disabled={loading || !!loadError || !!syntaxError || saving}
        onclick={save}
      >
        {saving ? "保存中…" : "保存"}
      </button>
    </footer>
  </div>
</div>
