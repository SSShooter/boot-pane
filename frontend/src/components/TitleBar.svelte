<script lang="ts">
  import { onMount } from "svelte";
  import { Window } from "@wailsio/runtime";

  // 只在无边框窗口里渲染，App.svelte 用 customChrome 控制。
  // 左格和侧栏同宽同底色，品牌只出现在这里；右边整段是拖拽区，最右是窗口
  // 按钮——应用名在整棵界面上只出现这一次。

  let maximised = $state(false);

  async function syncMaximised(): Promise<void> {
    try {
      maximised = await Window.IsMaximised();
    } catch {
      // 窗口正在关闭时再查状态会失败，这里没什么可做的，保持原样即可。
    }
  }

  onMount(() => {
    void syncMaximised();
    // 最大化/还原都会让 webview 改变尺寸，用 resize 一次覆盖所有来源
    // （双击标题栏、系统菜单、快捷键），不必逐个订阅窗口事件。
    window.addEventListener("resize", syncMaximised);
    return () => window.removeEventListener("resize", syncMaximised);
  });

  async function toggleMaximise(): Promise<void> {
    if (await Window.IsMaximised()) {
      await Window.Restore();
    } else {
      await Window.Maximise();
    }
  }

  /** 双击标题栏等于最大化/还原；但点在窗口按钮上不该触发。 */
  function onDblClick(event: MouseEvent): void {
    if ((event.target as HTMLElement).closest(".win-ctrls")) return;
    void toggleMaximise();
  }
</script>

<div class="titlebar" role="presentation" ondblclick={onDblClick}>
  <div class="titlebar-brand">
    <img class="brand-mark" src="/favicon.png" alt="" />
    <span class="brand-name">bootpane</span>
  </div>

  <!-- 中间整段放空：按住拖动由 runtime 认到 --wails-draggable 后转给系统拖窗，
       双击最大化没人替我们做，得自己接在 onDblClick 上。 -->
  <div class="titlebar-drag"></div>

  <div class="win-ctrls">
    <button class="win-btn" title="最小化" aria-label="最小化" onclick={() => void Window.Minimise()}>
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
        <path d="M1 5.5h8" stroke="currentColor" stroke-width="1.1" />
      </svg>
    </button>
    <button
      class="win-btn"
      title={maximised ? "向下还原" : "最大化"}
      aria-label={maximised ? "向下还原" : "最大化"}
      onclick={() => void toggleMaximise()}
    >
      <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.1" aria-hidden="true">
        {#if maximised}
          <rect x="1.4" y="2.9" width="5.6" height="5.6" rx="0.6" />
          <path d="M3.6 2.9V1.9a0.6 0.6 0 0 1 0.6-0.6h4a0.6 0.6 0 0 1 0.6 0.6v4a0.6 0.6 0 0 1-0.6 0.6H7" />
        {:else}
          <rect x="1.5" y="1.5" width="7" height="7" rx="0.8" />
        {/if}
      </svg>
    </button>
    <button class="win-btn win-close" title="关闭" aria-label="关闭" onclick={() => void Window.Close()}>
      <!-- 走的是和系统叉号一样的关窗流程：WindowClosing 钩子照样会拦下来问一句，
           有任务在跑时不会直接退掉。 -->
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
        <path d="M1.2 1.2l7.6 7.6M8.8 1.2L1.2 8.8" stroke="currentColor" stroke-width="1.1" />
      </svg>
    </button>
  </div>
</div>
