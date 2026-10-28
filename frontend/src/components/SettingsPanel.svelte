<script lang="ts">
  import { tick } from "svelte";
  import { app } from "../lib/state.svelte";
  import { OPEN_WITH_CUSTOM, OPEN_WITH_PRESETS, openWithLabel } from "../lib/openwith";
  import type { OpenWithPreset } from "../lib/openwith";
  import Select from "./Select.svelte";

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
    { value: "", label: "系统文件管理器" },
    ...choices.list,
    { value: OPEN_WITH_CUSTOM, label: "自定义…" },
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
      <h1>设置</h1>
      <div class="panel-path">本机偏好，存在 workspace.json，不写进仓库配置</div>
    </div>
  </div>

  <section class="group">
    <div class="group-head">
      <span class="group-name">运行</span>
      <span class="group-rule"></span>
    </div>

    <div class="setting-list">
      <div class="setting-row">
        <div class="setting-text">
          <div class="setting-label">启动任务时自动打开日志</div>
          <p class="setting-desc">
            点「启动」后自动展开这个任务的日志抽屉。已经开着别的任务的面板时不会切走；
            批量启动（星标任务）也不逐个弹。
          </p>
        </div>
        <button
          class="switch"
          class:is-on={autoOpenLog}
          role="switch"
          aria-checked={autoOpenLog}
          aria-label="启动任务时自动打开日志"
          onclick={() => app.setAutoOpenLogOnStart(!autoOpenLog)}
        >
          <span class="switch-knob"></span>
        </button>
      </div>
    </div>
  </section>

  <section class="group">
    <div class="group-head">
      <span class="group-name">打开方式</span>
      <span class="group-rule"></span>
    </div>

    <div class="setting-list">
      <div class="setting-row">
        <div class="setting-text">
          <div class="setting-label">打开项目目录时使用</div>
          <p class="setting-desc">
            项目头部的「打开目录」按钮会用它打开当前项目。下拉框里只列本机扫描到的编辑器，
            也可以填自定义命令：已经在 PATH 里的程序写名字即可，需要指定位置时用
            <code>{"{path}"}</code> 代表项目目录；不写占位符时目录会追加到命令末尾。
          </p>
        </div>
        <div class="setting-control">
          <Select
            value={showCustom ? OPEN_WITH_CUSTOM : currentOpenWith}
            options={openWithOptions}
            label="打开项目目录时使用"
            onchange={pickOpenWith}
          />

          {#if showCustom}
            <input
              class="text-input"
              type="text"
              aria-label="自定义打开命令"
              placeholder={"例如 code 或 code -n {path}"}
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
          <div class="setting-label">本机检测到的编辑器</div>
          <p class="setting-desc">
            {#if detected.length > 0}
              扫到了 {detected.length} 个，来自 PATH 和常见安装目录。刚装完新的编辑器可以再扫一次。
            {:else}
              一个也没扫到，上面的列表退回内置预设；刚装完编辑器也可以再扫一次。
            {/if}
          </p>
        </div>
        <button class="btn" onclick={() => app.detectEditors()}>重新扫描</button>
      </div>
    </div>
  </section>

  <section class="group">
    <div class="group-head">
      <span class="group-name">本机配置</span>
      <span class="group-rule"></span>
    </div>

    <div class="setting-list">
      <div class="setting-row">
        <div class="setting-text">
          <div class="setting-label">配置文件</div>
          <p class="setting-desc">
            项目列表、启动项和本页的偏好都存在 <code>workspace.json</code> 里。仓库自带的
            <code>.bootpane.json</code> 是另一份，只在该项目切到「项目内」时生效。
          </p>
        </div>
        <button class="btn" onclick={() => app.openConfigDir()}>打开配置目录</button>
      </div>
    </div>
  </section>
</div>
