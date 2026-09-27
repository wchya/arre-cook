const recipeText = require("./recipe-text")
const importFieldKeys = ["name", "ingredientsText", "seasoningsText", "stepsText", "remark", "cookTime"]
function readImportedRecipe(raw) {
  try {
    if (!raw || typeof raw !== "string" || raw.length > 60000) return {}
    const value = JSON.parse(raw), result = {}
    if (!value || typeof value !== "object") return {}
    for (const key of ["ingredients", "seasonings"]) {
      const items = value[key]
      if (Array.isArray(items) && items.length <= 25 && items.every((item) => item && typeof item.name === "string" && typeof item.amount === "string")) result[key] = items
    }
    if (Array.isArray(value.steps) && value.steps.length <= 30 && value.steps.every((item) => item && typeof item.text === "string" && Number.isFinite(item.time))) result.steps = value.steps
    return result
  } catch (_) { return {} }
}
function recipeImportPatch(start, current, recipe, touched = []) {
  const patch = {}, originals = readImportedRecipe(current.importedRecipeJSON)
  const empty = (key) => touched.indexOf(key) < 0 && !start[key].trim() && !current[key].trim()
  if (empty("name")) patch.name = recipe.name
  if (empty("remark") && recipe.remark) patch.remark = recipe.remark
  if (touched.indexOf("cookTime") < 0 && start.cookTime === 0 && current.cookTime === 0 && recipe.cook_time > 0) patch.cookTime = recipe.cook_time
  if (empty("ingredientsText") && recipe.ingredients.length) {
    originals.ingredients = recipe.ingredients.map(({ name, amount }) => ({ name, amount: amount || "" }))
    patch.ingredientsText = recipeText.formatIngredients(originals.ingredients)
  }
  if (empty("seasoningsText") && recipe.seasonings.length) {
    originals.seasonings = recipe.seasonings.map(({ name, amount }) => ({ name, amount: amount || "" }))
    patch.seasoningsText = recipeText.formatIngredients(originals.seasonings)
  }
  if (empty("stepsText") && recipe.steps.length) {
    originals.steps = recipe.steps.map(({ text, time }) => ({ text, time: time || 0 }))
    patch.stepsText = recipeText.formatSteps(originals.steps)
  }
  if (patch.ingredientsText || patch.seasoningsText || patch.stepsText) patch.importedRecipeJSON = JSON.stringify(originals)
  return patch
}
module.exports = { readImportedRecipe, recipeImportPatch, importFieldKeys }
