import { useQuery } from "@tanstack/react-query"
import { ArrowUpRight, Bot, Clock3, Flame, Leaf, Sparkles } from "lucide-react"
import { useNavigate } from "react-router-dom"
import { profileApi } from "@/api"
import PageHeader from "@/components/PageHeader"
import { cardShadow } from "@/components/Card"
import type { WeightItem } from "@/types"

function WeightList({ title, items, color }: { title: string; items: WeightItem[]; color: "orange" | "green" }) {
  const max = Math.max(1, ...items.map((item) => item.weight))
  return (
    <section className="border-b border-border py-4 last:border-b-0">
      <h2 className="mb-3 text-sm font-bold">{title}</h2>
      {items.length === 0 ? <p className="text-xs text-text3">累积一些用餐记录后会显示趋势</p> : (
        <div className="space-y-2.5">
          {items.slice(0, 6).map((item) => (
            <div key={item.name} className="grid grid-cols-[76px_1fr_40px] items-center gap-2 text-xs">
              <span className="truncate font-medium text-text2">{item.name}</span>
              <span className="h-2 overflow-hidden rounded-full bg-bg">
                <span className={`block h-full rounded-full ${color === "orange" ? "bg-primary" : "bg-mint"}`} style={{ width: `${Math.max(4, item.weight / max * 100)}%` }} />
              </span>
              <span className="text-right tabular-nums text-text3">{item.count} 次</span>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

export default function TasteProfile() {
  const navigate = useNavigate()
  const { data: profile, isLoading, isError } = useQuery({ queryKey: ["profile", 90], queryFn: () => profileApi.get(90) })
  return (
    <div className="min-h-dvh bg-bg pb-8">
      <PageHeader title="口味画像" subtitle="根据你自己的用餐记录整理" icon={Bot} onBack={() => navigate(-1)} />
      <main className="mx-auto max-w-[640px] space-y-4 px-5 py-5">
        {isLoading ? <div className="h-40 animate-pulse rounded-lg bg-card" /> : isError || !profile ? (
          <div role="alert" className="rounded-lg border border-border bg-card p-5 text-sm text-text2">画像暂时无法加载，请稍后重试。</div>
        ) : (
          <>
            <section className={`rounded-lg border border-border bg-card p-5 ${cardShadow}`}>
              <div className="flex items-center gap-2 text-xs font-semibold text-text3"><Sparkles size={14} className="text-primary" />近 {profile.window_days} 天</div>
              <p className="mt-3 text-[15px] leading-7 text-text">{profile.summary || "继续记录每一餐，画像会逐渐贴近你的口味。"}</p>
              <div className="mt-5 grid grid-cols-3 divide-x divide-border border-y border-border py-3 text-center">
                <div><div className="text-xl font-bold tabular-nums">{profile.window_records}</div><div className="text-[11px] text-text3">餐次</div></div>
                <div><div className="text-xl font-bold tabular-nums">{profile.distinct_dishes}</div><div className="text-[11px] text-text3">不同菜品</div></div>
                <div><div className="text-xl font-bold tabular-nums">{profile.avg_cook_time > 0 ? `${Math.round(profile.avg_cook_time)} 分` : "-"}</div><div className="text-[11px] text-text3">平均用时</div></div>
              </div>
              <div className="mt-4 flex flex-wrap gap-x-5 gap-y-2 text-xs text-text2">
                <span className="inline-flex items-center gap-1.5"><Flame size={14} className="text-primary" />辣味占比 {Math.round(profile.spicy_ratio * 100)}%</span>
                <span className="inline-flex items-center gap-1.5"><Leaf size={14} className="text-mint" />避开过敏与忌口食材</span>
                <span className="inline-flex items-center gap-1.5"><Clock3 size={14} />推荐去重 {profile.repeat_days} 天</span>
              </div>
            </section>

            <section className={`rounded-lg border border-border bg-card px-4 ${cardShadow}`}>
              <WeightList title="常选口味" items={profile.taste_weights} color="orange" />
              <WeightList title="常吃菜系" items={profile.category_weights} color="green" />
              <div className="py-4">
                <h2 className="mb-3 text-sm font-bold">最近常吃</h2>
                {profile.top_dishes.length === 0 ? <p className="text-xs text-text3">还没有足够的用餐记录</p> : profile.top_dishes.slice(0, 5).map((dish) => (
                  <button key={dish.dish_id} onClick={() => navigate(`/dishes/${dish.dish_id}`)} className="flex w-full items-center justify-between border-t border-border py-3 text-left text-sm first:border-0">
                    <span className="truncate font-medium">{dish.dish_name}</span><span className="ml-3 flex shrink-0 items-center gap-1 text-xs text-text3">{dish.count} 次<ArrowUpRight size={13} /></span>
                  </button>
                ))}
              </div>
              <div className="border-t border-border py-4">
                <h2 className="mb-2 text-sm font-bold">需要避开</h2>
                {profile.disliked_dishes.length === 0 ? <p className="text-xs text-text3">暂无明确不喜欢的菜品</p> : <div className="flex flex-wrap gap-2">{profile.disliked_dishes.map((dish) => <span key={dish.id} className="rounded-md bg-bg px-2.5 py-1 text-xs text-text2">{dish.name}</span>)}</div>}
              </div>
            </section>
            <button onClick={() => navigate("/me/preferences")} className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-text text-sm font-semibold text-white transition-colors hover:bg-text/90">调整饮食偏好<ArrowUpRight size={15} /></button>
          </>
        )}
      </main>
    </div>
  )
}
