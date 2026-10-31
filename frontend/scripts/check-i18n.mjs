// 检查四份字典的 key 和占位符是否一致。
//
// key 少一条 TypeScript 就会报错，但占位符漏了不会：英文写了 {count}、中文
// 只写了数字时，界面上会安静地少一个变量。改完文案跑一次：
//   npm run check:i18n
import { readFileSync } from "node:fs";

const dir = new URL("../src/lib/i18n/", import.meta.url);
const langs = ["en", "zh", "pt", "ru"];

function load(lang) {
  const src = readFileSync(new URL(`${lang}.ts`, dir), "utf8");
  const re = /"([\w.]+)":\s*("(?:[^"\\]|\\.)*")/g;
  const map = new Map();
  let m;
  while ((m = re.exec(src))) {
    map.set(m[1], JSON.parse(m[2]));
  }
  return map;
}

const base = load("en");
let bad = 0;

for (const lang of langs.slice(1)) {
  const dict = load(lang);
  for (const key of base.keys()) {
    if (!dict.has(key)) {
      console.error(`${lang}: 缺少 ${key}`);
      bad++;
    }
  }
  for (const key of dict.keys()) {
    if (!base.has(key)) {
      console.error(`${lang}: 多出 ${key}`);
      bad++;
    }
  }
}

const vars = (s) => [...s.matchAll(/\{(\w+)\}/g)].map((x) => x[1]).sort().join(",");

for (const lang of langs) {
  const dict = load(lang);
  for (const [key, msg] of dict) {
    const want = vars(base.get(key) ?? "");
    if (vars(msg) !== want) {
      console.error(`${lang}: ${key} 占位符不一致，英文 ${want || "无"}，本语言 ${vars(msg) || "无"}`);
      bad++;
    }
  }
}

console.log(bad === 0 ? `OK：${base.size} 条文案，四种语言一致` : `发现 ${bad} 处不一致`);
process.exit(bad === 0 ? 0 : 1);
