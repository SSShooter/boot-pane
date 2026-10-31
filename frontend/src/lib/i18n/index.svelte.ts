import en from "./en";
import pt from "./pt";
import ru from "./ru";
import zh from "./zh";
import type { MessageKey } from "./en";

export type { MessageKey };

/**
 * 支持的语言短码。
 *
 * 和 Go 侧 internal/i18n 的 Lang 是同一套值：前端切语言会把它原样传给
 * ConfigService.SetLanguage，后端生成的错误信息和校验提示才跟着一起变。
 */
export type Locale = "zh" | "en" | "pt" | "ru";

const catalogs: Record<Locale, Record<MessageKey, string>> = { zh, en, pt, ru };

/** 设置页语言下拉框的候选项，名字用各语言自己的写法，找自己那门语言最快。 */
export const LOCALES: { value: Locale; label: string }[] = [
  { value: "zh", label: "简体中文" },
  { value: "en", label: "English" },
  { value: "pt", label: "Português" },
  { value: "ru", label: "Русский" },
];

/**
 * 把系统语言、配置里存的值这类外部输入收敛成支持的短码。
 *
 * 认不出来就返回空串，交给调用方决定怎么兜底——静默落到某个语言会让
 * 「设置没生效」看起来像 bug。
 */
export function normalizeLocale(raw: string | null | undefined): Locale | "" {
  const base = (raw ?? "").trim().toLowerCase().split(/[-_]/)[0];
  switch (base) {
    case "zh":
      return "zh";
    case "en":
      return "en";
    case "pt":
      return "pt";
    case "ru":
      return "ru";
    default:
      return "";
  }
}

/**
 * 当前语言。
 *
 * 初始值跟系统语言走，认不出来时用英文兜底：首次启动时 store 里还没有存过
 * 语言，这时候先按浏览器语言显示，state.loadSettings 再把它落盘。
 *
 * 必须是 $state：t() 读它，模板和派生值因此会在切语言时自动重算。
 */
const initial: Locale =
  normalizeLocale(typeof navigator === "undefined" ? "" : navigator.language) || "en";

let current = $state<Locale>(initial);

function applyDocumentLang(locale: Locale): void {
  if (typeof document !== "undefined") document.documentElement.lang = locale;
}

// 首帧前对齐 <html lang>；之后由 setLocale 维护。这里用非响应式的初始值，
// 免得把模块顶层的读取卷进响应式依赖。
applyDocumentLang(initial);

export function currentLocale(): Locale {
  return current;
}

/** 切换语言；不认识的语言原样保留当前值并返回它。 */
export function setLocale(raw: string): Locale {
  const next = normalizeLocale(raw);
  if (!next) return current;
  if (next !== current) {
    current = next;
    applyDocumentLang(next);
  }
  return next;
}

const VAR_RE = /\{(\w+)\}/g;

/**
 * 取一条文案，`{name}` 形式的占位符会被 vars 里的同名值替换。
 *
 * 缺语言或缺 key 时依次回落到英文、key 本身，界面上顶多出现一个英文单词，
 * 不会整块空白。没有对应变量的占位符原样保留，方便一眼看出漏传了什么。
 */
export function t(key: MessageKey, vars?: Record<string, string | number>): string {
  const message = catalogs[current][key] ?? en[key] ?? key;
  if (!vars) return message;
  return message.replace(VAR_RE, (whole, name: string) =>
    Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : whole,
  );
}
