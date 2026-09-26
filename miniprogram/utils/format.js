const WEEKDAYS = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]

function pad(value) {
  return String(value).padStart(2, "0")
}

function dateKey(date) {
  const d = date || new Date()
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function addDays(date, days) {
  const d = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  d.setDate(d.getDate() + days)
  return d
}

// iOS 不认 "2026-09-26 12:00:00" 这类格式，统一转成斜杠再解析。
function parseDate(value) {
  if (!value) return null
  if (value instanceof Date) return value
  const text = String(value)
  const dateOnly = /^\d{4}-\d{2}-\d{2}$/.test(text)
  const parsed = new Date(dateOnly ? text.replace(/-/g, "/") : text)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

function greeting() {
  const hour = new Date().getHours()
  if (hour < 6) return "夜深了~"
  if (hour < 11) return "早上好 ☀️"
  if (hour < 14) return "中午好 🍳"
  if (hour < 18) return "下午好 ☀️"
  return "晚上好 🌙"
}

function weekday(value) {
  const d = parseDate(value)
  return d ? WEEKDAYS[d.getDay()] : ""
}

function relativeDate(value) {
  const d = parseDate(value)
  if (!d) return ""
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const target = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const diff = Math.floor((today.getTime() - target.getTime()) / 86400000)
  if (diff === 0) return "今天"
  if (diff === 1) return "昨天"
  if (diff === 2) return "前天"
  if (diff > 0 && diff < 7) return `${diff}天前`
  if (diff > 0 && diff < 30) return `${Math.floor(diff / 7)}周前`
  if (d.getFullYear() === now.getFullYear()) return `${d.getMonth() + 1}月${d.getDate()}日`
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

function shortDateTime(value) {
  const d = parseDate(value)
  if (!d) return ""
  return `${d.getMonth() + 1}月${d.getDate()}日 ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function monthDay(value) {
  const d = parseDate(value)
  return d ? `${d.getMonth() + 1}月${d.getDate()}日` : ""
}

function clock(seconds) {
  const s = Math.max(0, Math.floor(seconds))
  return `${Math.floor(s / 60)}:${pad(s % 60)}`
}

module.exports = { WEEKDAYS, pad, dateKey, addDays, parseDate, greeting, weekday, relativeDate, shortDateTime, monthDay, clock }
