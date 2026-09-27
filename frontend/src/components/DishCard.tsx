import { Heart } from "lucide-react"
import type { Dish } from "@/types"
import DishImage from "@/components/DishImage"
import { cardShadow } from "@/components/Card"

interface DishCardProps {
  dish: Dish
  onClick?: () => void
  showFav?: boolean
  favActive?: boolean
  onToggleFav?: () => void
  showDifficulty?: boolean
}

export default function DishCard({ dish, onClick, showFav, favActive, onToggleFav, showDifficulty }: DishCardProps) {
  return (
    <article className={`relative min-w-0 overflow-hidden rounded-2xl border border-border bg-card ${cardShadow}`}>
      <button type="button" onClick={onClick} className="block w-full text-left transition-colors hover:bg-bg active:bg-primary-light" aria-label={`查看${dish.name}`}>
        <div className="relative h-[132px] bg-primary-light">
          <DishImage dish={dish} className="h-full w-full" />
          {showDifficulty && <span className="absolute bottom-2 left-2 rounded-lg bg-card/95 px-2 py-1 text-[11px] font-medium text-text2">{dish.difficulty === "easy" ? "简单" : dish.difficulty === "medium" ? "中等" : "困难"}</span>}
        </div>
        <div className="px-3 py-3">
          <h3 className="mb-1 truncate text-sm font-semibold">{dish.name}</h3>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-text2"><span>{dish.category || "家常菜"}</span><span aria-hidden="true">·</span><span>{dish.cook_time > 0 ? `${dish.cook_time} 分钟` : "时间待补充"}</span></div>
        </div>
      </button>
      {showFav && <button type="button" onClick={onToggleFav} aria-pressed={Boolean(favActive)} aria-label={`${favActive ? "取消收藏" : "收藏"}${dish.name}`}
        className={`absolute right-1 top-1 flex h-11 w-11 items-center justify-center rounded-full border border-border bg-card/95 shadow-sm transition-colors ${favActive ? "text-primary" : "text-text2"}`}>
        <Heart size={19} fill={favActive ? "currentColor" : "none"} />
      </button>}
    </article>
  )
}
