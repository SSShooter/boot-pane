<script lang="ts">
  import { app } from "../lib/state.svelte";
  import { t } from "../lib/i18n/index.svelte";

  const req = $derived(app.confirmRequest);

  /**
   * 正文和逐行清单是两个节点，aria-describedby 得把用到的 id 都带上。
   */
  const describedBy = $derived(
    [req?.body ? "confirm-body" : "", req?.items?.length ? "confirm-items" : ""]
      .filter(Boolean)
      .join(" ") || undefined,
  );

  let cancelBtn = $state<HTMLButtonElement | null>(null);

  /**
   * 默认聚焦「取消」，关掉后把焦点还给原来的按钮。
   *
   * 确认框几乎都是破坏性操作，焦点落在确认按钮上时，一个误按的回车
   * 就把事办了。effect 读 cancelBtn 是为了在元素挂载时再聚焦一次。
   */
  $effect(() => {
    if (!req) return;
    const prev = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cancelBtn?.focus();
    return () => {
      if (prev?.isConnected) prev.focus();
    };
  });

  function onKeydown(e: KeyboardEvent): void {
    if (!req) return;
    // 确认框盖在最上层，Esc 只应该关它，不能再往下传给编辑器之类的窗口级监听。
    if (e.key === "Escape") {
      e.preventDefault();
      app.resolveConfirm(false);
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if req}
  <div class="overlay overlay-confirm">
    <div
      class="dialog dialog-confirm"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      aria-describedby={describedBy}
    >
      <header class="dialog-head">
        <h2 id="confirm-title">{req.title}</h2>
      </header>

      <div class="dialog-body confirm-body">
        {#if req.body}
          <p class="confirm-text" id="confirm-body">{req.body}</p>
        {/if}
        {#if req.items?.length}
          <ul class="confirm-list" id="confirm-items">
            {#each req.items as item}
              <li>{item}</li>
            {/each}
          </ul>
        {/if}
        {#if req.detail}
          <p class="confirm-detail">{req.detail}</p>
        {/if}
      </div>

      <footer class="dialog-foot">
        <button class="btn" bind:this={cancelBtn} onclick={() => app.resolveConfirm(false)}>
          {req.cancelLabel ?? t("confirm.cancel")}
        </button>
        <button
          class="btn {req.danger ? 'btn-danger-solid' : 'btn-primary'}"
          onclick={() => app.resolveConfirm(true)}
        >
          {req.confirmLabel ?? t("confirm.ok")}
        </button>
      </footer>
    </div>
  </div>
{/if}
