import type { Dish } from "@/types"
import DishImage from "@/components/DishImage"
import { cardShadow } from "@/components/Card"

function diffLabel(d: string) { return d === "easy" ? "简单" : d === "medium" ? "中等" : "困难" }
function diffColor(d: string) { return d === "easy" ? "text-mint" : d === "medium" ? "text-yellow" : "text-primary" }

interface DishCardProps {
  dish: Dish
  onClick?: () => void
  /** 是否显示收藏爱心角标 */
  showFav?: boolean
  /** 当前是否已收藏 */
  favActive?: boolean
  /** 点击爱心回调（已 stopPropagation） */
  onToggleFav?: () => void
  /** 是否显示左下难度角标 */
  showDifficulty?: boolean
}

// 菜品网格卡：Home 心情推荐 / DishList 主网格复用
export default function DishCard({ dish, onClick, showFav, favActive, onToggleFav, showDifficulty }: DishCardProps) {
  return (
    <button
      onClick={onClick}
      className={`bg-card rounded-2xl overflow-hidden border border-border ${cardShadow} transition-all active:scale-97 text-left w-full`}
    >
      <div className="h-[120px] bg-gradient-to-br from-primary-light to-pink-light relative">
        <DishImage dish={dish} className="w-full h-full" />
        {showFav && (
          <span
            onClick={(e) => { e.stopPropagation(); onToggleFav?.() }}
            className={`absolute top-2 right-2 w-7 h-7 rounded-full bg-card/90 backdrop-blur-sm flex items-center justify-center text-sm shadow-sm transition-all active:scale-90 ${favActive ? "text-primary animate-heartbeat" : "text-text3"}`}
          >
            {favActive ? "❤" : "♡"}
          </span>
        )}
        {showDifficulty && (
          <span className={`absolute bottom-2 left-2 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-card/90 backdrop-blur-sm ${diffColor(dish.difficulty)}`}>
            {diffLabel(dish.difficulty)}
          </span>
        )}
      </div>
      <div className="px-3 py-2.5">
        <div className="text-sm font-semibold truncate mb-1">{dish.name}</div>
        <div className="flex gap-1.5 items-center text-[11px] text-text2">
          {dish.category} <span className="w-[3px] h-[3px] rounded-full bg-text3" /> {dish.cook_time}分钟
        </div>
      </div>
    </button>
  )
}
