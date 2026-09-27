function ingredientLine(item) {
  return typeof item === "string" ? item : `${item.name} ${item.amount || ""}`.trim()
}
function formatIngredients(items) { return items.map(ingredientLine).join("\n") }
function parseIngredients(text, previous = []) {
  if (text === formatIngredients(previous)) return previous
  return text.split("\n").map((line) => line.trim()).filter(Boolean).map((line) => {
    const original = previous.find((item) => ingredientLine(item) === line)
    if (original !== undefined) return original
    const parts = line.split(/\s+/)
    return { name: parts.length === 1 ? parts[0] : parts.slice(0, -1).join(" "), amount: parts.length === 1 ? "" : parts[parts.length - 1] }
  })
}
function formatSteps(items) {
  return items.map((item, index) => `${index + 1}. ${typeof item === "string" ? item : `${item.text}${item.time ? ` (${item.time}分钟)` : ""}`}`).join("\n")
}
function parseSteps(text, previous = []) {
  // Editing a name or cover must not remove step photos or split multiline steps.
  if (text === formatSteps(previous)) return previous
  const lines = text.split("\n").map((line) => line.trim()).filter(Boolean)
  return lines.map((line, index) => {
    const cleaned = line.replace(/^\d+\.\s*/, "")
    const timing = /\((\d+(?:\.\d+)?)\s*分钟\)/
    const match = cleaned.match(timing)
    const step = { text: cleaned.replace(timing, "").trim(), time: match ? Number(match[1]) : 0 }
    const matched = previous.find((item) => (typeof item === "string" ? item : item.text) === step.text)
    const original = matched !== undefined ? matched : previous.length === lines.length ? previous[index] : undefined
    if (original && typeof original !== "string" && original.image) step.image = original.image
    return step
  })
}
module.exports = { formatIngredients, parseIngredients, formatSteps, parseSteps }
