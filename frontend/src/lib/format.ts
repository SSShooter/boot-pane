/** 日志面板最多保留的字符数，和后端环形缓冲的量级对齐。 */
export const MAX_LOG_CHARS = 200_000;

/**
 * 终端控制序列。
 *
 * ConPTY 输出的是完整的终端字节流：颜色、清屏、光标移动、窗口标题、spinner 重绘。
 * 直接塞进 <pre> 会看到满屏 \x1b[32m 之类的垃圾。
 */
// eslint-disable-next-line no-control-regex
const ANSI_PATTERN = /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]/g;

/**
 * 去掉 ANSI 转义序列，返回净化后的文本和「尚未收完的尾巴」。
 *
 * 转义序列可能被切在两个数据块之间（`\x1b[3` + `2m`），所以末尾的不完整
 * 序列要留到下一次拼接再处理，否则会在日志里留下半截乱码。
 */
export function stripAnsi(chunk: string): { clean: string; carry: string } {
  // eslint-disable-next-line no-control-regex
  const tail = /(\x1b(?:\[[0-?]*[ -/]*|\][^\x07\x1b]*)?)$/.exec(chunk);

  let head = chunk;
  let carry = "";
  // 尾巴太长说明那根本不是转义序列，是正常文本里恰好有个 ESC，直接放行。
  if (tail && tail[1].length <= 24) {
    carry = tail[1];
    head = chunk.slice(0, tail.index);
  }
  return { clean: head.replace(ANSI_PATTERN, ""), carry };
}

/**
 * 处理行内回车。
 *
 * vite 的 spinner 靠不断写 \r 重绘同一行，全保留会刷出几十行重复内容。
 * 每行只留最后一个 \r 之后的部分，等于只显示最终状态。
 *
 * 注意先归一化 \r\n，否则 Windows 的换行会被误判成行内覆盖。
 */
export function collapseCarriageReturns(text: string): string {
  if (!text.includes("\r")) return text;
  return text
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((line) => {
      const idx = line.lastIndexOf("\r");
      return idx >= 0 ? line.slice(idx + 1) : line;
    })
    .join("\n");
}

/** 百分比：个位数保留一位小数，否则取整，避免「1%」「2%」看不出波动。 */
export function formatPercent(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0%";
  return value < 10 ? `${value.toFixed(1)}%` : `${Math.round(value)}%`;
}

/** 字节数：按 KB / MB / GB 自适应。 */
export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0";
  const kb = value / 1024;
  if (kb < 1024) return `${Math.round(kb)}KB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb < 10 ? mb.toFixed(1) : Math.round(mb)}MB`;
  return `${(mb / 1024).toFixed(2)}GB`;
}
