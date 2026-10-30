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

/** 同一套兜底逻辑，macOS 版。runtime 环境值是 "darwin"。 */
function onMac(): boolean {
  const os = (window as any)?._wails?.environment?.OS;
  if (typeof os === "string") return os === "darwin";
  return /Macintosh/i.test(navigator.userAgent);
}

/**
 * 是否由前端自己画标题栏。
 *
 * 必须和 main.go 里窗口的 Frameless 判断逐字对齐：Windows 走无边框 + 自绘
 * （components/TitleBar.svelte），macOS 保留系统交通灯，只在顶部留出隐形
 * 标题栏的高度。浏览器预览里没有窗口 API，永远按系统外观渲染。
 */
export const customChrome = !isPreview && onWindows();

/**
 * 浏览器预览里强制看 macOS 顶部留白：加 ?mac=1。
 * 纯调试入口，浏览器里没有真的交通灯，只用来核对布局和品牌位置。
 */
const macPreviewOverride =
  typeof location !== "undefined" &&
  new URLSearchParams(location.search).has("mac");

/**
 * 是否跑在 macOS 窗口里。
 *
 * macOS 保留系统交通灯（main.go 用 MacTitleBarHiddenInset），前端不画窗口
 * 按钮，但要补一行顶部留白把内容压到隐形拖拽区之下，并在交通灯右边把
 * 品牌露出来——否则应用名在整个界面上无处可寻。
 */
export const macChrome = macPreviewOverride || (!isPreview && onMac());
