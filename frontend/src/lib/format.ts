/** 日志面板最多保留的字符数，和后端环形缓冲的量级对齐。 */
export const MAX_LOG_CHARS = 200_000;

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
