import type { DishIngredient, DishStep } from "@/types"
import type { VideoRecipe } from "@/api/video-recipe"
import { formatIngredients, formatSteps } from "./recipe-text"

export interface ImportFields {
  name: string; ingredientsText: string; seasoningsText: string; stepsText: string; remark: string; cookTime: number; importedRecipeJSON?: string
}
export interface ImportedRecipe { ingredients?: DishIngredient[]; seasonings?: DishIngredient[]; steps?: DishStep[] }
export const importFieldKeys = ["name", "ingredientsText", "seasoningsText", "stepsText", "remark", "cookTime"] as const
export type ImportFieldKey = typeof importFieldKeys[number]

export function readImportedRecipe(raw?: string): ImportedRecipe {
  try {
    if (!raw || raw.length > 60000) return {}
    const value = JSON.parse(raw) as ImportedRecipe
    if (!value || typeof value !== "object") return {}
    const result: ImportedRecipe = {}
    for (const key of ["ingredients", "seasonings"] as const) {
      const items = value[key]
      if (Array.isArray(items) && items.length <= 25 && items.every((item) => item && typeof item.name === "string" && typeof item.amount === "string")) result[key] = items
    }
    if (Array.isArray(value.steps) && value.steps.length <= 30 && value.steps.every((item) => item && typeof item.text === "string" && Number.isFinite(item.time))) result.steps = value.steps
    return result
  } catch { return {} }
}

// Structured originals survive text display, saving and draft restoration.
export function recipeImportPatch(start: ImportFields, current: ImportFields, recipe: VideoRecipe, touched: ImportFieldKey[] = []): Partial<ImportFields> {
  const patch: Partial<ImportFields> = {}
  const originals = readImportedRecipe(current.importedRecipeJSON)
  const empty = (key: Exclude<ImportFieldKey, "cookTime">) => !touched.includes(key) && !start[key].trim() && !current[key].trim()
  if (empty("name")) patch.name = recipe.name
  if (empty("remark") && recipe.remark) patch.remark = recipe.remark
  if (!touched.includes("cookTime") && start.cookTime === 0 && current.cookTime === 0 && recipe.cook_time > 0) patch.cookTime = recipe.cook_time
  if (empty("ingredientsText") && recipe.ingredients.length) {
    originals.ingredients = recipe.ingredients.map(({ name, amount }) => ({ name, amount: amount || "" }))
    patch.ingredientsText = formatIngredients(originals.ingredients)
  }
  if (empty("seasoningsText") && recipe.seasonings.length) {
    originals.seasonings = recipe.seasonings.map(({ name, amount }) => ({ name, amount: amount || "" }))
    patch.seasoningsText = formatIngredients(originals.seasonings)
  }
  if (empty("stepsText") && recipe.steps.length) {
    originals.steps = recipe.steps.map(({ text, time }) => ({ text, time: time || 0 }))
    patch.stepsText = formatSteps(originals.steps)
  }
  if (patch.ingredientsText || patch.seasoningsText || patch.stepsText) patch.importedRecipeJSON = JSON.stringify(originals)
  return patch
}
