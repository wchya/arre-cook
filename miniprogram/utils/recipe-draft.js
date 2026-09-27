// Drafts contain form text and uploaded URLs, never credentials or photo bytes.
const STRING_FIELDS = ["name", "category", "mealType", "difficulty", "ingredientsText", "seasoningsText", "stepsText", "remark", "imageUrl", "videoUrl"]
const ARRAY_FIELDS = ["tasteList", "images", "tags"]
const NUMBER_FIELDS = ["cookTime", "sortOrder"]

function key(userId, recipeId) { return `arre_recipe_draft:v1:${userId}:user:${recipeId || "new"}` }
function snapshot(data) {
  const value = {}
  STRING_FIELDS.forEach((field) => { value[field] = data[field] || "" })
  ARRAY_FIELDS.forEach((field) => { value[field] = (data[field] || []).slice() })
  NUMBER_FIELDS.forEach((field) => { value[field] = data[field] })
  value.shareFamily = Boolean(data.shareFamily)
  if (typeof data.importedRecipeJSON === "string" && data.importedRecipeJSON.length <= 60000) value.importedRecipeJSON = data.importedRecipeJSON
  return value
}
function read(userId, recipeId) {
  if (!userId) return null
  try {
    const draft = wx.getStorageSync(key(userId, recipeId))
    if (!draft || draft.version !== 1 || !Number.isFinite(draft.savedAt) || !draft.value) return null
    const value = draft.value
    if (STRING_FIELDS.some((field) => typeof value[field] !== "string")) return null
    if (NUMBER_FIELDS.some((field) => !Number.isFinite(value[field]))) return null
    if (ARRAY_FIELDS.some((field) => !Array.isArray(value[field]) || value[field].some((item) => typeof item !== "string"))) return null
    return { version: 1, savedAt: draft.savedAt, value: snapshot(value) }
  } catch (_) { return null }
}
function write(userId, recipeId, value) {
  if (!userId) return false
  try {
    wx.setStorageSync(key(userId, recipeId), { version: 1, savedAt: Date.now(), value: snapshot(value) })
    return true
  } catch (_) { return false }
}
function remove(userId, recipeId) {
  try { wx.removeStorageSync(key(userId, recipeId)); return true } catch (_) { return false }
}
module.exports = { key, snapshot, read, write, remove }
