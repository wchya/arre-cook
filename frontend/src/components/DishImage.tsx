import type { Dish } from "@/types"
import { getDishEmoji, getDishImageUrl } from "@/lib/dish-image"

interface DishImageProps {
  dish: Dish
  className?: string
  imgClassName?: string
  emojiSize?: string
}

export default function DishImage({ dish, className, imgClassName, emojiSize = "text-4xl" }: DishImageProps) {
  const url = getDishImageUrl(dish)
  if (url) {
    return (
      <div className={className}>
        <img src={url} alt={dish.name} className={`w-full h-full object-cover ${imgClassName || ""}`} />
      </div>
    )
  }
  return (
    <div className={`${className} flex items-center justify-center ${emojiSize}`}>
      {getDishEmoji(dish)}
    </div>
  )
}
