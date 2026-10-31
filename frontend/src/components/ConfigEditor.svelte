<script lang="ts">
  import type {
    Issue,
    ResolvedProject,
  } from "../../bindings/bootpane/internal/config/models";
  import { api, isPreview } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { t } from "../lib/i18n/index.svelte";

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
    if (!project.useRepoConfig)
      return { label: t("editor.targetGlobal"), path: "workspace.json" };
    return { label: t("editor.targetRepo"), path: `${project.path}\\.bootpane.json` };
  });

  /**
   * 前端只做语法预检。
   *
   * 真正的校验（工作目录存不存在、依赖指向的任务在不在）全在后端，
   * 这里提前拦住明显写坏的 JSON，省得白跑一趟还能给出更快的反馈。
   */
  const syntaxError = $derived.by(() => {
    if (!text.trim()) return t("editor.emptyContent");
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

  async function requestClose(): Promise<void> {
    if (saving) return;
    if (dirty) {
      const ok = await app.confirm({
        title: t("editor.unsavedTitle"),
        body: t("editor.unsavedBody"),
        confirmLabel: t("editor.discard"),
        cancelLabel: t("editor.keepEditing"),
        danger: true,
      });
      if (!ok) return;
    }
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
        errorMsg = res.error || t("editor.saveFailed");
        return;
      }
      await app.load();
      // 保存成功也可能带回 warn（比如端口重复），这些会在主界面的横幅里显示，
      // 这里只把条数点出来，免得用户以为「没报错就是全对」。
      app.notify(
        issues.length > 0
          ? t("editor.savedWithHints", { count: res.imported, hints: issues.length })
          : t("editor.saved", { count: res.imported }),
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
    // 确认框开着时键盘归它管：Esc 应该只关确认框，不能顺手把编辑器也关了。
    if (app.confirmOpen) return;
    if (e.key === "Escape") void requestClose();
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
        <h2 id="editor-title">{t("editor.title")}</h2>
        <p class="dialog-sub">
          {t("editor.saveTo", { target: target.label })}<code class="path-chip">{target.path}</code>
        </p>
      </div>
      <button class="icon-btn" aria-label={t("editor.close")} onclick={requestClose}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="6" y1="6" x2="18" y2="18" /><line x1="18" y1="6" x2="6" y2="18" />
        </svg>
      </button>
    </header>

    <div class="dialog-body editor-body">
      {#if loading}
        <div class="prompt-loading">{t("editor.loading")}</div>
      {:else if loadError}
        <div class="dialog-error">{loadError}</div>
      {:else}
        <div class="editor-bar">
          <button class="btn btn-mini" disabled={!!syntaxError} onclick={format}>
            {t("editor.format")}
          </button>
          {#if syntaxError}
            <span class="syntax-flag err">{t("editor.syntaxBad")}</span>
          {:else}
            <span class="syntax-flag ok">{t("editor.syntaxOk")}</span>
          {/if}
          <span class="editor-hint">{t("editor.hintSave")}</span>
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
          <div class="dialog-error">{t("editor.syntaxDetail", { error: syntaxError })}</div>
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
        <span class="dialog-note">{t("editor.previewNote")}</span>
      {:else if project.useRepoConfig}
        <span class="dialog-note">{t("editor.repoNote")}</span>
      {/if}
      <button class="btn" onclick={requestClose}>{t("editor.cancel")}</button>
      <button
        class="btn btn-primary"
        disabled={loading || !!loadError || !!syntaxError || saving}
        onclick={save}
      >
        {saving ? t("editor.saving") : t("editor.save")}
      </button>
    </footer>
  </div>
</div>
