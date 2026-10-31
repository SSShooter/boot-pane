<script lang="ts">
  import { Clipboard } from "@wailsio/runtime";
  import type {
    Issue,
    ResolvedProject,
  } from "../../bindings/bootpane/internal/config/models";
  import { api, isPreview } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { t } from "../lib/i18n/index.svelte";

  let { project, onclose }: { project: ResolvedProject; onclose: () => void } = $props();

  let prompt = $state("");
  let promptLoading = $state(true);
  let copied = $state(false);
  let pasted = $state("");
  let importing = $state(false);
  let errorMsg = $state("");
  let issues = $state<Issue[]>([]);

  $effect(() => {
    const id = project.id;
    promptLoading = true;
    errorMsg = "";
    api
      .buildConfigPrompt(id)
      .then((text) => {
        prompt = text;
      })
      .catch((e) => {
        errorMsg = describe(e);
      })
      .finally(() => {
        promptLoading = false;
      });
  });

  function describe(e: unknown): string {
    if (typeof e === "string") return e;
    if (e instanceof Error) return e.message;
    return String(e);
  }

  async function copyPrompt(): Promise<void> {
    try {
      await Clipboard.SetText(prompt);
    } catch {
      // 浏览器预览模式没有 Wails 剪贴板，退回浏览器 API
      await navigator.clipboard.writeText(prompt);
    }
    copied = true;
    setTimeout(() => {
      copied = false;
    }, 1800);
  }

  async function doImport(): Promise<void> {
    if (!pasted.trim()) return;
    importing = true;
    errorMsg = "";
    issues = [];
    try {
      const res = await api.importTasks(project.id, pasted);
      issues = res.issues ?? [];
      if (!res.ok) {
        errorMsg = res.error || t("gen.importFailed");
        return;
      }
      await app.load();
      app.notify(t("gen.imported", { count: res.imported }));
      onclose();
    } catch (e) {
      errorMsg = describe(e);
    } finally {
      importing = false;
    }
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === "Escape") onclose();
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="overlay">
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="gen-title">
    <header class="dialog-head">
      <div>
        <h2 id="gen-title">{t("gen.title")}</h2>
        <p class="dialog-sub">{t("gen.sub")}</p>
      </div>
      <button class="icon-btn" aria-label={t("gen.close")} onclick={onclose}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="6" y1="6" x2="18" y2="18" /><line x1="18" y1="6" x2="6" y2="18" />
        </svg>
      </button>
    </header>

    <div class="dialog-body">
      <section class="step">
        <div class="step-head">
          <span class="step-num">1</span>
          <span class="step-title">{t("gen.step1")}</span>
          <button class="btn btn-primary btn-mini" onclick={copyPrompt} disabled={promptLoading}>
            {copied ? t("gen.copied") : t("gen.copy")}
          </button>
        </div>
        <p class="step-hint">
          {t("gen.step1HintA")} <code>{project.path}</code>{t("gen.step1HintB")}
        </p>
        <div class="prompt-box">
          {#if promptLoading}
            <div class="prompt-loading">{t("gen.loading")}</div>
          {:else}
            <pre>{prompt}</pre>
          {/if}
        </div>
      </section>

      <section class="step">
        <div class="step-head">
          <span class="step-num">2</span>
          <span class="step-title">{t("gen.step2")}</span>
        </div>
        <textarea
          class="paste-area"
          bind:value={pasted}
          rows="7"
          spellcheck="false"
          placeholder={'{\n  "version": 1,\n  "project": { … },\n  "tasks": [ … ]\n}'}
        ></textarea>

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
      </section>
    </div>

    <footer class="dialog-foot">
      {#if isPreview}
        <span class="dialog-note">{t("gen.previewNote")}</span>
      {/if}
      <button class="btn" onclick={onclose}>{t("gen.cancel")}</button>
      <button class="btn btn-primary" disabled={!pasted.trim() || importing} onclick={doImport}>
        {importing ? t("gen.importing") : t("gen.import")}
      </button>
    </footer>
  </div>
</div>
