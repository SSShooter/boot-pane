<script lang="ts">
  import { app } from "../lib/state.svelte";
  import { OPEN_WITH_CUSTOM, OPEN_WITH_PRESETS } from "../lib/openwith";

  const autoOpenLog = $derived(app.settings.autoOpenLogOnStart);

  const currentOpenWith = $derived(app.openWith);
  const isPreset = $derived(OPEN_WITH_PRESETS.some((p) => p.value === currentOpenWith));

  /** 选中「自定义…」后输入框才出现；已保存的命令不在预设里时也要显示。 */
  let customMode = $state(false);
  let editing = $state(false);
  let draft = $state("");

  const showCustom = $derived(customMode || (!isPreset && currentOpenWith !== ""));

  // 不在编辑时让输入框跟着保存值走：从预设切过来时能直接看到当前命令。
  $effect(() => {
    if (!editing) draft = currentOpenWith;
  });

  function pickOpenWith(event: Event): void {
    const value = (event.currentTarget as HTMLSelectElement).value;
    if (value === OPEN_WITH_CUSTOM) {
      customMode = true;
      editing = true;
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
            项目头部的「打开目录」按钮会用它打开当前项目。可以直接选一个编辑器，也可以填自定义命令：
            已经在 PATH 里的程序写名字即可，需要指定位置时用 <code>{"{path}"}</code> 代表项目目录；
            不写占位符时目录会追加到命令末尾。
          </p>
        </div>
        <div class="setting-control">
          <select
            class="select"
            aria-label="打开项目目录时使用"
            value={showCustom ? OPEN_WITH_CUSTOM : currentOpenWith}
            onchange={pickOpenWith}
          >
            {#each OPEN_WITH_PRESETS as preset (preset.value)}
              <option value={preset.value}>{preset.label}</option>
            {/each}
            <option value={OPEN_WITH_CUSTOM}>自定义…</option>
          </select>

          {#if showCustom}
            <input
              class="text-input"
              type="text"
              aria-label="自定义打开命令"
              placeholder="例如 code 或 code -n {path}"
              value={draft}
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
