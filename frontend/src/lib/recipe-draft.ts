export interface RecipeDraftFields {
  name: string
  category: string
  mealType: string
  difficulty: string
  tasteList: string[]
  cookTime: number
  ingredientsText: string
  seasoningsText: string
  stepsText: string
  remark: string
  imageUrl: string
  images: string[]
  videoUrl: string
  tags: string[]
  sortOrder: number
  importedRecipeJSON?: string
}

export interface RecipeDraft { version: 1; savedAt: number; value: RecipeDraftFields }

export function recipeDraftKey(userId: number, recipeId: number, mode: string) {
  return `arre_recipe_draft:v1:${userId}:${mode}:${recipeId || "new"}`
}

export function readRecipeDraft(key: string): RecipeDraft | null {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(key) || "null")
    if (!raw || typeof raw !== "object") return null
    const draft = raw as Partial<RecipeDraft>
    if (draft.version !== 1 || !Number.isFinite(draft.savedAt) || !draft.value || typeof draft.value !== "object") return null
    const value = draft.value
    if (value.importedRecipeJSON !== undefined && (typeof value.importedRecipeJSON !== "string" || value.importedRecipeJSON.length > 60000)) return null
    const strings: (keyof RecipeDraftFields)[] = ["name", "category", "mealType", "difficulty", "ingredientsText", "seasoningsText", "stepsText", "remark", "imageUrl", "videoUrl"]
    if (strings.some((field) => typeof value[field] !== "string")) return null
    if (![value.cookTime, value.sortOrder].every(Number.isFinite)) return null
    if (![value.tasteList, value.images, value.tags].every((items) => Array.isArray(items) && items.every((item) => typeof item === "string"))) return null
    return draft as RecipeDraft
  } catch { return null }
}
