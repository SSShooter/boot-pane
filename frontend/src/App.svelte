<script lang="ts">
  import { onMount } from "svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import ProjectPanel from "./components/ProjectPanel.svelte";
  import StarredPanel from "./components/StarredPanel.svelte";
  import SettingsPanel from "./components/SettingsPanel.svelte";
  import GenerateDialog from "./components/GenerateDialog.svelte";
  import ConfigEditor from "./components/ConfigEditor.svelte";
  import ConfirmDialog from "./components/ConfirmDialog.svelte";
  import LogPanel from "./components/LogPanel.svelte";
  import { app } from "./lib/state.svelte";

  onMount(() => {
    // 先订阅再拉数据，否则「拉取期间刚好有进程状态变化」会漏掉。
    const disconnect = app.connect();
    void app.load();
    return disconnect;
  });
</script>

<div class="app">
  <Sidebar />

  <!-- 主内容区是纵向 flex：面板自己再分「固定头 + 滚动正文」两行，
       日志抽屉作为同一层的第三个兄弟排在最后，谁都不覆盖谁。 -->
  <main class="content">
    <div class="content-main">
      {#if app.showSettings}
        <SettingsPanel />
      {:else if app.showStarred}
        <StarredPanel />
      {:else if app.selected}
        <ProjectPanel project={app.selected} />
      {:else if !app.loading}
        <div class="empty">
          <div class="empty-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
              <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
            </svg>
          </div>
          {#if app.projects.length === 0}
            <h2>还没有添加项目</h2>
            <p>点击左上角的加号选择项目目录。项目会出现在这里，每个项目可以配置多个启动按钮。</p>
          {:else}
            <h2>选择左侧的一个项目</h2>
            <p>从列表里挑一个项目，右侧会显示它所有的启动按钮。</p>
          {/if}
        </div>
      {/if}
    </div>

    <!-- 面板弹到独立窗口后这里不再渲染它：同一块面板只能存在于一个地方，
         否则两边各显示一半状态，新日志也不知道该往哪边走。 -->
    {#if !app.logDetached}
      <LogPanel />
    {/if}
  </main>
</div>

{#if app.generatorProject}
  <GenerateDialog project={app.generatorProject} onclose={() => app.closeGenerator()} />
{/if}

{#if app.editorProject}
  <ConfigEditor project={app.editorProject} onclose={() => app.closeEditor()} />
{/if}

<ConfirmDialog />

{#if app.toast}
  <div class="toast" class:err={app.toastIsError} role="status" aria-live="polite">{app.toast}</div>
{/if}
