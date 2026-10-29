import { isPreview } from "./api";

/**
 * 当前是不是跑在 Windows 上。
 *
 * runtime 注入的 environment 可能晚于模块求值（自绘标题栏必须在首帧就定下来），
 * 所以这里拿不到就退回 UA 判断——两边结论必须一致，否则会落到「没有系统标题栏、
 * 也没有自绘标题栏」这种既不能拖也不能关的状态。
 */
function onWindows(): boolean {
  const os = (window as any)?._wails?.environment?.OS;
  if (typeof os === "string") return os === "windows";
  return /Windows NT/i.test(navigator.userAgent);
}

/**
 * 是否由前端自己画标题栏。
 *
 * 必须和 main.go 里窗口的 Frameless 判断逐字对齐：Windows 走无边框 + 自绘
 * （components/TitleBar.svelte），macOS 保留系统交通灯，只在顶部留出隐形
 * 标题栏的高度。浏览器预览里没有窗口 API，永远按系统外观渲染。
 */
export const customChrome = !isPreview && onWindows();
