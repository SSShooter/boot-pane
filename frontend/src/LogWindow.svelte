<script lang="ts">
  import { onMount } from "svelte";
  import LogPanel from "./components/LogPanel.svelte";
  import { app } from "./lib/state.svelte";

  // 先装状态再渲染：等 onMount 再装会先画一帧空面板，独立窗口刚弹出来
  // 那一瞬间看着像卡住了。
  app.adoptLogQuery(location.search);

  onMount(() => {
    // 以日志窗口的身份订阅：同一路 bootpane:log 事件在这里也收得到，
    // 但关窗确认这类主窗口的动作不归这里管。
    const disconnect = app.connect("log");
    void app.load();
    return disconnect;
  });
</script>

<main class="log-window">
  <LogPanel detached />
</main>
