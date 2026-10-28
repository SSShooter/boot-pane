/**
 * 「打开目录」用哪个程序。
 *
 * 设置里存的是一条完整命令行（空串表示系统文件管理器），这里只负责两件事：
 * 给设置页提供几个常用编辑器的一键选项，以及从命令行里认出「用什么打开」，
 * 用于按钮上的文案。
 */

export type OpenWithPreset = {
  /** 存进 workspace.json 的命令行。 */
  value: string;
  label: string;
};

export const OPEN_WITH_PRESETS: OpenWithPreset[] = [
  { value: "", label: "系统文件管理器" },
  { value: "code", label: "VS Code" },
  { value: "cursor", label: "Cursor" },
  { value: "idea", label: "IntelliJ IDEA" },
  { value: "webstorm", label: "WebStorm" },
];

/** 设置页里「自定义…」选项的值。不是一条真实命令，只在界面内部使用。 */
export const OPEN_WITH_CUSTOM = "__custom__";

/**
 * 从命令行里取出程序名，用于按钮文案。
 *
 * 只取第一个参数（去掉引号和 .exe/.cmd 后缀），所以
 * `"C:\...\Code.exe" -n` 会显示成 Code。认不出预设时就显示这个程序名，
 * 总比统一显示成「编辑器」更有信息量。
 */
export function openWithLabel(command: string): string {
  const trimmed = command.trim();
  if (!trimmed) return "系统文件管理器";

  const preset = OPEN_WITH_PRESETS.find((p) => p.value === trimmed);
  if (preset) return preset.label;

  const first = trimmed.match(/^"([^"]+)"|^(\S+)/);
  const raw = (first?.[1] ?? first?.[2] ?? trimmed).replace(/\\/g, "/");
  const base = raw.slice(raw.lastIndexOf("/") + 1).replace(/\.(exe|cmd|bat)$/i, "");
  return base || trimmed;
}
