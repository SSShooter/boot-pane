<script lang="ts">
  import { tick } from "svelte";
  import { app } from "../lib/state.svelte";
  import { OPEN_WITH_CUSTOM, OPEN_WITH_PRESETS, openWithLabel } from "../lib/openwith";
  import type { OpenWithPreset } from "../lib/openwith";
  import Select from "./Select.svelte";
  import { LOCALES, currentLocale, t } from "../lib/i18n/index.svelte";

  const autoOpenLog = $derived(app.settings.autoOpenLogOnStart);

  const detected = $derived(app.editors);
  const currentOpenWith = $derived(app.openWith);

  /**
   * 下拉框里的候选。
   *
   * 优先用扫描结果——用户要找的是「我装了的那个」，而不是一份写死的清单。
   * 一个都没扫到时退回内置预设（浏览器预览、或机器上确实什么都没装），
   * 免得下拉框只剩「系统文件管理器」和「自定义」两项。
   *
   * known 说的是「已保存的命令在候选里」：不在的话要把这条命令补成一个
   * 选项，否则下拉框会显示成空白，看起来像没设置过。
   */
  const choices = $derived.by(() => {
    const base: OpenWithPreset[] =
      detected.length > 0
        ? detected.map((e) => ({ value: e.command, label: e.name }))
        : OPEN_WITH_PRESETS;

    if (!currentOpenWith || base.some((o) => o.value === currentOpenWith)) {
      return { list: base, known: true };
    }
    return {
      list: [...base, { value: currentOpenWith, label: openWithLabel(currentOpenWith) }],
      known: false,
    };
  });

  /** 设置页那一行要选的东西：兜底的空值 + 候选 + 自定义。 */
  const openWithOptions = $derived<OpenWithPreset[]>([
    { value: "", label: t("settings.systemFileManager") },
    ...choices.list,
    { value: OPEN_WITH_CUSTOM, label: t("settings.custom") },
  ]);

  /** 选中「自定义…」后输入框才出现；已保存的命令不在候选里时也要显示。 */
  let customMode = $state(false);
  let editing = $state(false);
  let draft = $state("");
  let customInput = $state<HTMLInputElement | null>(null);

  const showCustom = $derived(customMode || (!choices.known && currentOpenWith !== ""));

  // 不在编辑时让输入框跟着保存值走：从预设切过来时能直接看到当前命令。
  $effect(() => {
    if (!editing) draft = currentOpenWith;
  });

  async function pickOpenWith(value: string): Promise<void> {
    if (value === OPEN_WITH_CUSTOM) {
      customMode = true;
      editing = true;
      // 选中「自定义…」就是来表达「我要写一条」，直接把光标送进输入框。
      await tick();
      customInput?.focus();
      return;
    }
    customMode = false;
    editing = false;
    void app.setOpenWith(value);
  }

  /** 提交自定义命令。回车和失焦都走这里，避免「改了没保存」的错觉。 */
  function commitCustom(): void {
    editing = false;
    customMode = false;
    void app.setOpenWith(draft);
  }
</script>

<div class="panel">
  <div class="panel-head">
    <div class="panel-title">
      <h1>{t("settings.title")}</h1>
      <div class="panel-path">{t("settings.subtitle")}</div>
    </div>
  </div>

  <div class="panel-body">
    <section class="group">
      <div class="group-head">
        <span class="group-name">{t("settings.groupLanguage")}</span>
        <span class="group-rule"></span>
      </div>

      <div class="setting-list">
        <div class="setting-row">
          <div class="setting-text">
            <div class="setting-label">{t("settings.languageLabel")}</div>
            <p class="setting-desc">{t("settings.languageDesc")}</p>
          </div>
          <div class="setting-control">
            <Select
              value={currentLocale()}
              options={LOCALES}
              label={t("settings.languageLabel")}
              onchange={(value) => void app.setLanguage(value)}
            />
          </div>
        </div>
      </div>
    </section>

    <section class="group">
      <div class="group-head">
        <span class="group-name">{t("settings.groupRuntime")}</span>
        <span class="group-rule"></span>
      </div>

      <div class="setting-list">
        <div class="setting-row">
          <div class="setting-text">
            <div class="setting-label">{t("settings.autoLogLabel")}</div>
            <p class="setting-desc">
              {t("settings.autoLogDesc")}
            </p>
          </div>
          <button
            class="switch"
            class:is-on={autoOpenLog}
            role="switch"
            aria-checked={autoOpenLog}
            aria-label={t("settings.autoLogLabel")}
            onclick={() => app.setAutoOpenLogOnStart(!autoOpenLog)}
          >
            <span class="switch-knob"></span>
          </button>
        </div>
      </div>
    </section>

    <section class="group">
      <div class="group-head">
        <span class="group-name">{t("settings.groupOpenWith")}</span>
        <span class="group-rule"></span>
      </div>

      <div class="setting-list">
        <div class="setting-row">
          <div class="setting-text">
            <div class="setting-label">{t("settings.openWithLabel")}</div>
            <p class="setting-desc">
              {t("settings.openWithDescA")}
              <code>{'{path}'}</code> {t("settings.openWithDescB")}
            </p>
          </div>
          <div class="setting-control">
            <Select
              value={showCustom ? OPEN_WITH_CUSTOM : currentOpenWith}
              options={openWithOptions}
              label={t("settings.openWithLabel")}
              onchange={pickOpenWith}
            />

            {#if showCustom}
              <input
                class="text-input"
                type="text"
                aria-label={t("settings.customAria")}
                placeholder={t("settings.customPlaceholder")}
                value={draft}
                bind:this={customInput}
                oninput={(e) => (draft = e.currentTarget.value)}
                onfocus={() => (editing = true)}
                onblur={commitCustom}
                onkeydown={(e) => {
                  if (e.key === "Enter") e.currentTarget.blur();
                }}
              />
            {/if}
          </div>
        </div>

        <div class="setting-row">
          <div class="setting-text">
            <div class="setting-label">{t("settings.detectedLabel")}</div>
            <p class="setting-desc">
              {#if detected.length > 0}
                {t("settings.detectedCount", { count: detected.length })}
              {:else}
                {t("settings.detectedNone")}
              {/if}
            </p>
          </div>
          <button class="btn" onclick={() => app.detectEditors()}>{t("settings.rescan")}</button>
        </div>
      </div>
    </section>

    <section class="group">
      <div class="group-head">
        <span class="group-name">{t("settings.groupConfig")}</span>
        <span class="group-rule"></span>
      </div>

      <div class="setting-list">
        <div class="setting-row">
          <div class="setting-text">
            <div class="setting-label">{t("settings.configLabel")}</div>
            <p class="setting-desc">
              {t("settings.configDescA")} <code>workspace.json</code> {t("settings.configDescB")}
              <code>.bootpane.json</code> {t("settings.configDescC")}
            </p>
          </div>
          <button class="btn" onclick={() => app.openConfigDir()}>{t("settings.openConfigDir")}</button>
        </div>
      </div>
    </section>
  </div>
</div>
