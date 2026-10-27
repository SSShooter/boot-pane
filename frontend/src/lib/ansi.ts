/**
 * 终端输出的着色解析。
 *
 * ConPTY 交过来的是完整终端字节流：既有 SGR 颜色，也有清屏、光标移动、
 * 窗口标题这类和「显示一行日志」无关的控制序列。这里只把 SGR 转成可渲染的
 * 片段，其余控制序列直接丢掉——否则 <pre> 里会露出 \x1b[32m 这样的乱码。
 *
 * 颜色不写死在这里：16 色走 CSS 变量（--ansi-0..15），配色在 style.css。
 * 浅色面板上照搬终端的「白色」等于隐形，必须按主题重新挑。
 */

/** 一段带样式的日志文本。 */
export type LogRun = {
  text: string;
  /** 语义类名（ansi-bold 等），空串表示没有附加样式。 */
  cls: string;
  /** 颜色等内联样式，空串表示继承默认。 */
  style: string;
};

type Flag = "bold" | "dim" | "italic" | "underline" | "strike" | "inverse";

/** index 是 0-255 的调色板下标，hex 用于真彩色。 */
type Color = { index: number } | { hex: string } | null;

/**
 * 单个任务的解析状态。
 *
 * 样式是跨数据块延续的（终端里一个颜色会一直生效到 reset），所以状态必须
 * 挂在任务上而不是每次调用重建。转义序列也可能被切在两块之间
 * （`\x1b[3` + `2m`），尾巴要留到下一次拼接再处理。
 */
export type AnsiState = {
  carry: string;
  fg: Color;
  bg: Color;
  flags: Set<Flag>;
  /** 当前样式的渲染结果，SGR 没变时复用，避免每个文本片段都现拼。 */
  cached: { cls: string; style: string } | null;
};

/** 类名固定按这个顺序拼，否则不同插入顺序会让本该合并的两段对不上。 */
const FLAG_ORDER = ["bold", "dim", "italic", "underline", "strike"] as const;

// 终端控制序列：OSC（窗口标题）、CSI（颜色 / 光标 / 清屏）、两字符 ESC。
// eslint-disable-next-line no-control-regex
const ANSI_RE = /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]/g;

// 末尾没写完的转义序列。
// eslint-disable-next-line no-control-regex
const TAIL_RE = /(\x1b(?:\[[0-?]*[ -/]*|\][^\x07\x1b]*)?)$/;

/** 尾巴超过这个长度就当成正常文本里恰好有个 ESC，不再等后续块。 */
const MAX_CARRY = 24;

export function createAnsiState(): AnsiState {
  return { carry: "", fg: null, bg: null, flags: new Set(), cached: null };
}

/** 解析一块日志输出，返回可以直接渲染的片段（行内回车已就地压掉）。 */
export function parseAnsiChunk(state: AnsiState, chunk: string): LogRun[] {
  if (!chunk) return [];

  const input = state.carry + chunk;
  const tail = TAIL_RE.exec(input);
  let head = input;
  if (tail && tail[1].length <= MAX_CARRY) {
    state.carry = tail[1];
    head = input.slice(0, tail.index);
  } else {
    state.carry = "";
  }

  const writer = new RunWriter();
  let last = 0;
  ANSI_RE.lastIndex = 0;
  for (let m = ANSI_RE.exec(head); m; m = ANSI_RE.exec(head)) {
    if (m.index > last) writer.push(head.slice(last, m.index), resolve(state));
    // 只有 CSI ... m 是 SGR，别的控制序列看到也当没看见。
    if (m[0].endsWith("m") && m[0].startsWith("\x1b[")) applySgr(state, m[0]);
    last = m.index + m[0].length;
  }
  if (last < head.length) writer.push(head.slice(last), resolve(state));

  return writer.runs;
}

/** 把新片段并进已有片段。相邻同一样式的要合并，否则节点数会按块数膨胀。 */
export function mergeRuns(base: LogRun[], extra: LogRun[]): LogRun[] {
  if (extra.length === 0) return base;

  const out = base.slice();
  let from = 0;
  const last = out[out.length - 1];
  const first = extra[0];
  if (last && last.cls === first.cls && last.style === first.style) {
    out[out.length - 1] = { ...last, text: last.text + first.text };
    from = 1;
  }
  for (let i = from; i < extra.length; i++) out.push(extra[i]);
  return out;
}

/** 片段里的字符总数，用来判断什么时候该裁。 */
export function countRuns(runs: readonly LogRun[]): number {
  let total = 0;
  for (const run of runs) total += run.text.length;
  return total;
}

/**
 * 超出上限时从头部裁剪。
 *
 * 切点尽量对齐到换行：从半行开始显示，用户会以为日志本身就少了一段。
 * 返回的 chars 是裁剪后的字符数，存回状态里，下次不用重算。
 */
export function trimRuns(
  runs: LogRun[],
  total: number,
  max: number,
): { runs: LogRun[]; chars: number } {
  if (total <= max) return { runs, chars: total };

  const target = total - max;
  // 找到 target 之后的第一处换行，把整行留下。
  let acc = 0;
  let cut = target;
  search: for (const run of runs) {
    let from = 0;
    for (;;) {
      const nl = run.text.indexOf("\n", from);
      if (nl < 0) break;
      if (acc + nl + 1 >= target) {
        cut = acc + nl + 1;
        break search;
      }
      from = nl + 1;
    }
    acc += run.text.length;
  }

  let dropped = 0;
  let index = 0;
  while (index < runs.length && dropped + runs[index].text.length <= cut) {
    dropped += runs[index].text.length;
    index++;
  }

  const out = runs.slice(index);
  if (index < runs.length && cut > dropped) {
    out[0] = { ...runs[index], text: runs[index].text.slice(cut - dropped) };
  }
  return { runs: out, chars: total - cut };
}

/**
 * 逐块拼装片段。
 *
 * 行内回车（spinner / 进度条重绘同一行）必须在片段层面处理：直接删字符会把
 * 颜色标记留在原地，后面整段文本的样式就错位了。这里的做法是记住当前行的
 * 起始偏移，遇到裸 \r 就把这一行已经写的内容整体回退掉。
 */
class RunWriter {
  runs: LogRun[] = [];
  private total = 0;
  private lineStart = 0;

  push(text: string, style: { cls: string; style: string }): void {
    let start = 0;
    for (let i = 0; i < text.length; i++) {
      const ch = text[i];
      if (ch !== "\n" && ch !== "\r") continue;

      if (i > start) this.append(text.slice(start, i), style);
      if (ch === "\n") {
        this.append("\n", style);
        this.lineStart = this.total;
      } else if (text[i + 1] !== "\n") {
        // 裸 \r 表示重写当前行。\r\n 里的 \r 交给下一轮的 \n 处理，
        // 否则 Windows 换行会被当成行内覆盖。
        this.truncate(this.lineStart);
      }
      start = i + 1;
    }
    if (start < text.length) this.append(text.slice(start), style);
  }

  private append(text: string, style: { cls: string; style: string }): void {
    const last = this.runs[this.runs.length - 1];
    if (last && last.cls === style.cls && last.style === style.style) {
      last.text += text;
    } else {
      this.runs.push({ text, cls: style.cls, style: style.style });
    }
    this.total += text.length;
  }

  private truncate(offset: number): void {
    while (this.total > offset && this.runs.length > 0) {
      const last = this.runs[this.runs.length - 1];
      const over = this.total - offset;
      if (last.text.length <= over) {
        this.total -= last.text.length;
        this.runs.pop();
      } else {
        last.text = last.text.slice(0, last.text.length - over);
        this.total = offset;
      }
    }
  }
}

function applySgr(state: AnsiState, seq: string): void {
  // seq 形如 \x1b[1;31m，去掉 ESC[ 和结尾的 m。
  const body = seq.slice(2, -1);
  const codes = body === "" ? [0] : body.split(";").map((p) => Number(p) || 0);

  for (let i = 0; i < codes.length; i++) {
    const c = codes[i];

    if (c === 38 || c === 48) {
      const mode = codes[i + 1];
      if (mode === 5 && i + 2 < codes.length) {
        setColor(state, c, { index: codes[i + 2] });
        i += 2;
      } else if (mode === 2 && i + 4 < codes.length) {
        setColor(state, c, { hex: hex(codes[i + 2], codes[i + 3], codes[i + 4]) });
        i += 4;
      } else {
        // 缺参数的扩展色，认不出来就跳过。
        i += 1;
      }
      continue;
    }

    switch (c) {
      case 0:
        state.fg = null;
        state.bg = null;
        state.flags.clear();
        break;
      case 1:
        state.flags.add("bold");
        break;
      case 2:
        state.flags.add("dim");
        break;
      case 3:
        state.flags.add("italic");
        break;
      case 4:
        state.flags.add("underline");
        break;
      case 7:
        state.flags.add("inverse");
        break;
      case 9:
        state.flags.add("strike");
        break;
      case 21:
      case 22:
        state.flags.delete("bold");
        state.flags.delete("dim");
        break;
      case 23:
        state.flags.delete("italic");
        break;
      case 24:
        state.flags.delete("underline");
        break;
      case 27:
        state.flags.delete("inverse");
        break;
      case 29:
        state.flags.delete("strike");
        break;
      case 39:
        state.fg = null;
        break;
      case 49:
        state.bg = null;
        break;
      default:
        if (c >= 30 && c <= 37) state.fg = { index: c - 30 };
        else if (c >= 40 && c <= 47) state.bg = { index: c - 40 };
        else if (c >= 90 && c <= 97) state.fg = { index: c - 90 + 8 };
        else if (c >= 100 && c <= 107) state.bg = { index: c - 100 + 8 };
        break;
    }
  }

  state.cached = null;
}

function setColor(state: AnsiState, code: number, color: Color): void {
  if (code === 38) state.fg = color;
  else state.bg = color;
}

function resolve(state: AnsiState): { cls: string; style: string } {
  if (state.cached) return state.cached;

  const parts: string[] = [];
  if (state.flags.has("inverse")) {
    // 反显：前景背景互换，缺省的一侧用面板底色 / 正文色补上。
    parts.push(`color:${state.bg ? cssColor(state.bg) : "var(--surface)"}`);
    parts.push(`background-color:${state.fg ? cssColor(state.fg) : "var(--text)"}`);
  } else {
    if (state.fg) parts.push(`color:${cssColor(state.fg)}`);
    if (state.bg) parts.push(`background-color:${cssColor(state.bg)}`);
  }

  const cls = FLAG_ORDER.filter((f) => state.flags.has(f))
    .map((f) => `ansi-${f}`)
    .join(" ");

  state.cached = { cls, style: parts.join(";") };
  return state.cached;
}

function cssColor(color: Exclude<Color, null>): string {
  if ("hex" in color) return color.hex;
  return color.index < 16 ? `var(--ansi-${color.index})` : xterm256(color.index);
}

/** xterm 256 色里 16-255 的标准取值：6×6×6 色立方 + 24 级灰阶。 */
const CUBE = [0, 95, 135, 175, 215, 255];

function xterm256(index: number): string {
  if (index >= 232) {
    const v = 8 + (index - 232) * 10;
    return hex(v, v, v);
  }
  const i = index - 16;
  return hex(CUBE[Math.floor(i / 36) % 6], CUBE[Math.floor(i / 6) % 6], CUBE[i % 6]);
}

function hex(r: number, g: number, b: number): string {
  return `#${[r, g, b].map((v) => clamp(v).toString(16).padStart(2, "0")).join("")}`;
}

function clamp(v: number): number {
  if (!Number.isFinite(v)) return 0;
  return Math.max(0, Math.min(255, Math.round(v)));
}
