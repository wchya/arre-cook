import type { DishIngredient, DishStep } from "@/types"

type Ingredients = Array<string | DishIngredient>
type Steps = Array<string | DishStep>

function ingredientLine(item: string | DishIngredient) {
  return typeof item === "string" ? item : `${item.name} ${item.amount || ""}`.trim()
}

export function formatIngredients(items: Ingredients) {
  return items.map(ingredientLine).join("\n")
}

export function parseIngredients(text: string, previous: Ingredients = []): Ingredients {
  if (text === formatIngredients(previous)) return previous
  return text.split("\n").map((line) => line.trim()).filter(Boolean).map((line) => {
    const original = previous.find((item) => ingredientLine(item) === line)
    if (original !== undefined) return original
    const parts = line.split(/\s+/)
    return { name: parts.length === 1 ? parts[0] : parts.slice(0, -1).join(" "), amount: parts.length === 1 ? "" : parts.at(-1) }
  })
}

export function formatSteps(items: Steps) {
  return items.map((item, index) => `${index + 1}. ${typeof item === "string" ? item : `${item.text}${item.time ? ` (${item.time}分钟)` : ""}`}`).join("\n")
}

export function parseSteps(text: string, previous: Steps = []): Steps {
  // Retain multiline instructions and metadata when a different field was edited.
  if (text === formatSteps(previous)) return previous
  const lines = text.split("\n").map((line) => line.trim()).filter(Boolean)
  return lines.map((line, index) => {
    const cleaned = line.replace(/^\d+\.\s*/, "")
    const timing = /\((\d+(?:\.\d+)?)\s*分钟\)/
    const match = cleaned.match(timing)
    const step: DishStep = { text: cleaned.replace(timing, "").trim(), time: match ? Number(match[1]) : 0 }
    const original = previous.find((item) => (typeof item === "string" ? item : item.text) === step.text)
      ?? (previous.length === lines.length ? previous[index] : undefined)
    if (original && typeof original !== "string" && original.image) step.image = original.image
    return step
  })
}
