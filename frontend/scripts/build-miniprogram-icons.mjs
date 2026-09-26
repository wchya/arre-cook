// 把 Web 端使用的 lucide 图标导出给微信小程序（小程序无法直接用 lucide-react）。
// 用法：cd frontend && node scripts/build-miniprogram-icons.mjs
// 输出：miniprogram/utils/icons.js —— 只包含下面列出的图标，新增图标后重新运行即可。
import { writeFileSync } from "node:fs"
import path from "node:path"
import { fileURLToPath, pathToFileURL } from "node:url"

const ICONS = [
  // 导航
  "house", "utensils-crossed", "sparkles", "calendar-days", "user-round",
  "arrow-left", "arrow-right", "arrow-up", "chevron-right", "chevron-left", "chevron-down", "chevron-up",
  // 操作
  "x", "plus", "minus", "check", "search", "trash-2", "pencil", "square-pen", "share-2", "copy",
  "refresh-cw", "rotate-ccw", "settings", "sliders-horizontal", "ellipsis", "loader-circle", "external-link",
  "download", "upload", "link",
  // 状态与账号
  "circle-check", "circle-alert", "info", "triangle-alert", "shield-check", "shield", "lock", "key-round",
  "mail", "eye", "eye-off", "smartphone", "log-out", "bell", "inbox", "badge-check",
  // 美食
  "chef-hat", "cooking-pot", "soup", "salad", "flame", "leaf", "egg", "fish", "carrot", "wheat", "utensils", "coffee",
  // 功能
  "heart", "star", "clock", "clock-3", "timer", "history", "message-circle", "message-square", "send",
  "book-open", "notebook-pen", "calendar", "calendar-check", "calendar-clock", "list-checks", "clipboard-list",
  "shopping-basket", "shopping-cart", "images", "image", "camera", "trophy", "bot", "plug", "activity",
  "users-round", "user-plus", "layout-dashboard", "gift", "dices", "lightbulb", "moon", "sun", "sunrise",
  "play", "pause", "skip-forward", "skip-back", "volume-2", "video", "wand-sparkles", "thumbs-up",
  "thumbs-down", "target", "trending-up", "chart-pie", "chart-column", "file-text", "crown", "zap", "hand-heart", "tag",
  "wrench", "heart-pulse", "arrow-up-right", "flag", "mail-plus", "user-minus", "house-plus", "door-open",
]

// 非 lucide 的品牌图标（实心绘制）。微信标识路径来自 Simple Icons（CC0）。
const FILLED = {
  wechat: '<path fill-rule="evenodd" d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 0 1 .213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.326.326 0 0 0 .167-.054l1.903-1.114a.864.864 0 0 1 .717-.098 10.16 10.16 0 0 0 2.837.403c.276 0 .543-.027.811-.05-.857-2.578.157-4.972 1.932-6.446 1.703-1.415 3.882-1.98 5.853-1.838-.576-3.583-4.196-6.348-8.596-6.348zM5.785 5.991c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178A1.17 1.17 0 0 1 4.623 7.17c0-.651.52-1.18 1.162-1.18zm5.813 0c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178 1.17 1.17 0 0 1-1.162-1.178c0-.651.52-1.18 1.162-1.18zm5.34 2.867c-1.797-.052-3.746.512-5.28 1.786-1.72 1.428-2.687 3.72-1.78 6.22.942 2.453 3.666 4.229 6.884 4.229.826 0 1.622-.12 2.361-.336a.722.722 0 0 1 .598.082l1.584.926a.272.272 0 0 0 .14.047c.134 0 .24-.111.24-.247 0-.06-.023-.12-.038-.177l-.327-1.233a.582.582 0 0 1-.023-.156.49.49 0 0 1 .201-.398C23.024 18.48 24 16.82 24 14.98c0-3.21-2.931-5.837-6.656-6.088V8.89c-.135-.01-.27-.027-.407-.03zm-2.53 3.274c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.97-.982zm4.844 0c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.969-.982z"/>',
}

const here = path.dirname(fileURLToPath(import.meta.url))
const iconDir = path.resolve(here, "../node_modules/lucide-react/dist/esm/icons")
const output = path.resolve(here, "../../miniprogram/utils/icons.js")

function escapeAttr(value) {
  return String(value).replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;")
}

function renderNode([tag, attrs]) {
  const pairs = Object.entries(attrs)
    .filter(([name]) => name !== "key")
    .map(([name, value]) => `${name.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)}="${escapeAttr(value)}"`)
  return `<${tag} ${pairs.join(" ")}/>`
}

const stroke = {}
const missing = []
for (const name of ICONS) {
  try {
    const mod = await import(pathToFileURL(path.join(iconDir, `${name}.mjs`)).href)
    stroke[name] = mod.__iconNode.map(renderNode).join("")
  } catch {
    missing.push(name)
  }
}

const lines = [
  "// 由 frontend/scripts/build-miniprogram-icons.mjs 从 lucide-react 生成，请勿手动修改。",
  "module.exports = {",
  "  stroke: {",
  ...Object.entries(stroke).map(([name, body]) => `    ${JSON.stringify(name)}: ${JSON.stringify(body)},`),
  "  },",
  "  filled: {",
  ...Object.entries(FILLED).map(([name, body]) => `    ${JSON.stringify(name)}: ${JSON.stringify(body)},`),
  "  },",
  "}",
  "",
]
writeFileSync(output, lines.join("\n"))
console.log(`wrote ${Object.keys(stroke).length} stroke icons + ${Object.keys(FILLED).length} filled icons to ${path.relative(process.cwd(), output)}`)
if (missing.length) {
  console.error(`missing lucide icons: ${missing.join(", ")}`)
  process.exitCode = 1
}
