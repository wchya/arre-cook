const { asArray, dishCover, isImageUrl } = require("./media")

// 与 Web 端 lib/dish-image.ts、DishCard、DishDetail 中的映射保持一致。
const CATEGORY_EMOJI = { 川菜: "🌶", 湘菜: "🔥", 贵州菜: "🍲", 云南菜: "🍄", 粤菜: "🐟" }
const DIFFICULTY = {
  easy: { label: "简单", tone: "mint" },
  medium: { label: "中等", tone: "yellow" },
  hard: { label: "困难", tone: "primary" },
}
const MEAL_LABEL = { lunch: "🍳 午餐", dinner: "🍲 晚餐" }
const MOOD_EMOJI = { yum: "😋", ok: "😐", no: "😵", great: "😋", meh: "😕" }
const HOME_MOODS = [
  { key: "happy", emoji: "😊", label: "开心" },
  { key: "tired", emoji: "😫", label: "疲惫" },
  { key: "lazy", emoji: "😌", label: "想偷懒" },
  { key: "spicy", emoji: "🤤", label: "想吃辣" },
  { key: "healthy", emoji: "🌿", label: "想养生" },
]
const HOME_MOOD_MAP = HOME_MOODS.reduce((map, mood) => { map[mood.key] = mood; return map }, {})

function dishEmoji(dish) {
  if (!dish) return "🍽"
  if (CATEGORY_EMOJI[dish.category]) return CATEGORY_EMOJI[dish.category]
  if (dish.image_url && !isImageUrl(dish.image_url) && dish.image_url.length <= 4) return dish.image_url
  return "🍽"
}

function difficulty(value) {
  return DIFFICULTY[value] || DIFFICULTY.easy
}

function mealLabel(value) {
  return MEAL_LABEL[value] || "🍳🍲 午餐+晚餐"
}

function tasteTags(taste) {
  return String(taste || "").split(/[,，、\s]+/).map((item) => item.trim()).filter(Boolean)
}

function normalizeIngredients(raw) {
  return asArray(raw).map((item) => {
    if (typeof item === "string") return { name: item, amount: "" }
    if (item && typeof item === "object") return { name: String(item.name || ""), amount: String(item.amount || "") }
    return { name: String(item), amount: "" }
  }).filter((item) => item.name)
}

function normalizeSteps(raw) {
  return asArray(raw).map((item) => {
    if (typeof item === "string") return { text: item, time: 0, image: "" }
    if (item && typeof item === "object") {
      return { text: String(item.text || ""), time: Number(item.time) || 0, image: typeof item.image === "string" ? item.image : "" }
    }
    return { text: String(item), time: 0, image: "" }
  }).filter((item) => item.text)
}

// 列表 / 卡片用的精简视图模型。
function toCard(dish) {
  if (!dish) return null
  const diff = difficulty(dish.difficulty)
  return {
    id: dish.id,
    name: dish.name,
    category: dish.category || "家常菜",
    cookTime: dish.cook_time || 0,
    difficulty: dish.difficulty || "easy",
    diffLabel: diff.label,
    diffTone: diff.tone,
    taste: dish.taste || "",
    tasteTags: tasteTags(dish.taste),
    cover: dishCover(dish),
    emoji: dishEmoji(dish),
    favorite: Boolean(dish.favorite),
    mealType: dish.meal_type || "all",
    isPrivate: Boolean(dish.owner_id) && !dish.family_id,
    isFamily: Boolean(dish.family_id),
  }
}

function toCards(list) {
  return (list || []).map(toCard).filter(Boolean)
}

module.exports = {
  CATEGORY_EMOJI,
  MOOD_EMOJI,
  HOME_MOODS,
  HOME_MOOD_MAP,
  dishEmoji,
  difficulty,
  mealLabel,
  tasteTags,
  normalizeIngredients,
  normalizeSteps,
  toCard,
  toCards,
}
