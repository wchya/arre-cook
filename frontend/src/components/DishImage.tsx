import { useState } from "react"
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
  const [failedUrl, setFailedUrl] = useState<string | null>(null)

  if (url && failedUrl !== url) {
    return (
      <div className={className}>
        <img
          src={url}
          alt={dish.name}
          loading="lazy"
          decoding="async"
          onError={() => setFailedUrl(url)}
          className={`w-full h-full object-cover ${imgClassName || ""}`}
        />
      </div>
    )
  }
  return (
    <div className={`${className} flex items-center justify-center ${emojiSize} bg-primary-light`}>
      <span role="img" aria-label={`${dish.name} 厨师图标`}>{getDishEmoji(dish)}</span>
    </div>
  )
}
