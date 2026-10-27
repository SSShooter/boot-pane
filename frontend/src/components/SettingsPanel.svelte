<script lang="ts">
  import { app } from "../lib/state.svelte";

  const autoOpenLog = $derived(app.settings.autoOpenLogOnStart);
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
