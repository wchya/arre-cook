import type { Dish } from "@/types"
import { asArray } from "@/lib/utils"

const categoryEmojis: Record<string, string> = {
  川菜: "🌶", 湘菜: "🔥", 贵州菜: "🍲", 云南菜: "🍄", 粤菜: "🐟",
}

export function getDishEmoji(dish: Dish): string {
  return categoryEmojis[dish.category] || dish.image_url || "🍽"
}

export function isImageUrl(url: string | undefined): boolean {
  if (!url) return false
  return url.startsWith("/uploads/") || url.startsWith("http") || /\.(jpg|jpeg|png|webp|gif|svg)(\?|$)/i.test(url)
}

export function getDishImageUrl(dish: Dish): string | null {
  if (isImageUrl(dish.image_url)) return dish.image_url
  const images = asArray<string>(dish.images)
  if (images.length > 0 && typeof images[0] === "string" && isImageUrl(images[0])) return images[0]
  return null
}
