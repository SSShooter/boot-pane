import { Window } from "@wailsio/runtime";
import { isPreview } from "./api";

/**
 * 关掉当前窗口。
 *
 * 桌面壳里走 Wails 的窗口 API；浏览器预览里没有这层东西，退回 window.close()
 * ——预览下独立窗口是用 window.open 开的，脚本开的窗口自己还能关掉。
 *
 * 调用后窗口不一定立刻消失，后续代码不能再假设自己还活着。
 */
export function closeCurrentWindow(): void {
  if (isPreview) {
    window.close();
    return;
  }
  void Window.Close().catch(() => window.close());
}
