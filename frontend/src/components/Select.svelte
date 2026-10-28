<script lang="ts">
  /**
   * 通用下拉选择。
   *
   * 原生 <select> 在 Windows 上是系统绘制的：边框、箭头、展开的列表都跟这套界面
   * 不是一路，高度也对不齐 30px 的按钮行。这里用按钮 + listbox 自己画一版，
   * 键盘（上下、Home/End、Enter、Esc）和 ARIA 按 combobox 的约定补齐。
   *
   * 菜单用 fixed 定位而不是绝对定位：设置页在滚动容器里，绝对定位会被裁掉。
   * 所以打开时量一次触发按钮的位置，滚动和缩放时跟着重算。
   */

  type SelectOption = {
    value: string;
    label: string;
  };

  let {
    value,
    options,
    label,
    onchange,
  }: {
    value: string;
    options: SelectOption[];
    label: string;
    onchange: (value: string) => void;
  } = $props();

  const id = $props.id();

  let open = $state(false);
  /** 键盘高亮的那一项，和选中项是两回事：上下移动但没回车前不落盘。 */
  let active = $state(0);
  /** 菜单的定位样式，量完尺寸才有值；空串表示还没摆好，先藏起来。 */
  let style = $state("");

  let trigger = $state<HTMLButtonElement | null>(null);
  let menu = $state<HTMLUListElement | null>(null);
  let items = $state<HTMLLIElement[]>([]);

  const selected = $derived(options.find((o) => o.value === value));
  const activeId = $derived(open ? `${id}-opt-${active}` : undefined);

  function place(): void {
    if (!trigger || !menu) return;
    const box = trigger.getBoundingClientRect();
    const width = Math.max(box.width, menu.offsetWidth);
    const height = menu.offsetHeight;

    // 下面不够高且上面更宽裕时往上翻，免得贴着窗口底边被切掉一截。
    const below = window.innerHeight - box.bottom;
    const up = below < height + 8 && box.top > below;

    // 右边缘对齐触发按钮，再夹回窗口内。
    const left = Math.min(
      Math.max(box.right - width, 8),
      Math.max(window.innerWidth - width - 8, 8),
    );

    style = [
      `min-width: ${Math.round(box.width)}px`,
      `left: ${Math.round(left)}px`,
      `top: ${Math.round(up ? box.top - height - 6 : box.bottom + 6)}px`,
    ].join("; ");
  }

  /**
   * 打开后才量尺寸：菜单是 {#if open} 里挂上来的，effect 在 DOM 更新之后才跑，
   * 这时候 offsetHeight 才是有值的。
   */
  $effect(() => {
    if (!open) {
      style = "";
      return;
    }
    place();
    reveal();

    window.addEventListener("scroll", place, true);
    window.addEventListener("resize", place);
    return () => {
      window.removeEventListener("scroll", place, true);
      window.removeEventListener("resize", place);
    };
  });

  function reveal(): void {
    items[active]?.scrollIntoView({ block: "nearest" });
  }

  function openMenu(): void {
    active = Math.max(
      0,
      options.findIndex((o) => o.value === value),
    );
    open = true;
  }

  function close(refocus = true): void {
    open = false;
    if (refocus) trigger?.focus();
  }

  function choose(index: number): void {
    const option = options[index];
    if (!option) return;
    close();
    // 选回同一项不用再落盘一次。
    if (option.value !== value) onchange(option.value);
  }

  function onKeydown(e: KeyboardEvent): void {
    switch (e.key) {
      case "ArrowDown":
      case "ArrowUp": {
        e.preventDefault();
        if (!open) {
          openMenu();
          return;
        }
        const step = e.key === "ArrowDown" ? 1 : -1;
        active = (active + step + options.length) % options.length;
        reveal();
        return;
      }
      case "Home":
      case "End": {
        if (!open) return;
        e.preventDefault();
        active = e.key === "Home" ? 0 : options.length - 1;
        reveal();
        return;
      }
      case "Enter":
      case " ": {
        e.preventDefault();
        if (open) choose(active);
        else openMenu();
        return;
      }
      case "Escape": {
        if (!open) return;
        // 菜单盖在页面内容上，Esc 只应该收起它，不要再传给窗口级监听。
        e.preventDefault();
        e.stopPropagation();
        close();
        return;
      }
      case "Tab": {
        if (open) close(false);
        return;
      }
    }
  }

  /**
   * 点菜单和点按钮以外的任何地方都收起。
   *
   * 按钮自己走 onclick 切换，所以这里要放过它，否则先关后开，点一下等于没点。
   */
  function onPointerDown(e: PointerEvent): void {
    if (!open) return;
    const target = e.target as Node | null;
    if (!target) return;
    if (trigger?.contains(target) || menu?.contains(target)) return;
    close(false);
  }

  /** 鼠标落在哪一项上。点击和悬停都借用它，两者共用一套高亮。 */
  function indexAt(target: EventTarget | null): number {
    const el = target instanceof Element ? target.closest<HTMLElement>("[data-index]") : null;
    const raw = el?.dataset.index;
    return raw === undefined ? -1 : Number(raw);
  }
</script>

<svelte:window onpointerdown={onPointerDown} />

<div class="select">
  <button
    class="select-trigger"
    class:is-open={open}
    type="button"
    role="combobox"
    aria-label={label}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-controls={open ? `${id}-menu` : undefined}
    aria-activedescendant={activeId}
    bind:this={trigger}
    onclick={() => (open ? close() : openMenu())}
    onkeydown={onKeydown}
  >
    <span class="select-value">{selected?.label ?? ""}</span>
    <svg
      class="select-caret"
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2.2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <polyline points="6 9 12 15 18 9" />
    </svg>
  </button>

  {#if open}
    <!--
      点击和悬停都挂在 listbox 上，不挂到每个选项上：焦点始终留在触发按钮，
      键盘交互走 aria-activedescendant，所以键盘处理在按钮的 onkeydown 里，
      这个 ul 自己不需要再响应键盘——下面这条 a11y 忽略说的是同一件事。
    -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <ul
      class="select-menu"
      class:is-placed={style !== ""}
      id={`${id}-menu`}
      role="listbox"
      aria-label={label}
      style={style}
      bind:this={menu}
      onclick={(e) => {
        const index = indexAt(e.target);
        if (index >= 0) choose(index);
      }}
      onpointermove={(e) => {
        const index = indexAt(e.target);
        if (index >= 0) active = index;
      }}
    >
      {#each options as option, index (option.value)}
        <li
          class="select-option"
          class:is-active={index === active}
          id={`${id}-opt-${index}`}
          role="option"
          aria-selected={option.value === value}
          data-index={index}
          bind:this={items[index]}
        >
          {option.label}
        </li>
      {/each}
    </ul>
  {/if}
</div>
