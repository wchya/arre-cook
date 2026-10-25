import { useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { pickApi, profileApi, mealPlansApi, settingsApi, recommendationFeedbackApi } from "@/api"
import type { RecommendItem, SmartPickRequest, TasteProfile, WeightItem } from "@/types"
import { asArray } from "@/lib/utils"
import { recommendationDate } from "@/lib/recommendation"
import { useRecommendationImpression } from "@/lib/use-recommendation-impression"
import DishImage from "@/components/DishImage"
import PageHeader from "@/components/PageHeader"
import { cardShadow } from "@/components/Card"
import toast from "react-hot-toast"
import { Bot, Clock, Flame, Leaf, RefreshCw, Sparkles, ThumbsDown, Zap, type LucideIcon } from "lucide-react"

type MealChoice = "" | "lunch" | "dinner"

const DEFAULT_TASTES = ["辣", "麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "酱香"]

const moodOptions: Array<{ key: string; label: string; Icon: LucideIcon }> = [
  { key: "", label: "随便", Icon: Sparkles },
  { key: "spicy", label: "想吃辣", Icon: Flame },
  { key: "tired", label: "省事点", Icon: Zap },
  { key: "healthy", label: "清爽点", Icon: Leaf },
]

const cookTimeOptions = [
  { value: 0, label: "不限" },
  { value: 20, label: "≤20 分钟" },
  { value: 35, label: "≤35 分钟" },
  { value: 60, label: "≤60 分钟" },
]

const mealOptions: Array<{ key: MealChoice; label: string }> = [
  { key: "", label: "不限餐次" },
  { key: "lunch", label: "🍳 午餐" },
  { key: "dinner", label: "🍲 晚餐" },
]

const homeMoodLabel: Record<string, string> = { happy: "开心", tired: "疲惫", lazy: "想偷懒", spicy: "想吃辣", healthy: "想养生" }

function diffLabel(d: string) {
  return d === "easy" ? "简单" : d === "medium" ? "中等" : "困难"
}

function parseList(raw: unknown, fallback: string[]): string[] {
  const arr = asArray<string>(raw).filter((x) => typeof x === "string")
  return arr.length > 0 ? arr : fallback
}

function pct(v: number) {
  return `${Math.round(v * 100)}%`
}

function WeightBars({ items, tone }: { items: WeightItem[]; tone: "primary" | "mint" }) {
  if (items.length === 0) return <div className="text-[12px] text-text3">还没有足够的记录</div>
  const max = Math.max(...items.map((i) => i.weight), 0.01)
  return (
    <div className="space-y-1.5">
      {items.slice(0, 5).map((it) => (
        <div key={it.name} className="flex items-center gap-2">
          <span className="w-12 shrink-0 truncate text-[12px] font-semibold text-text2">{it.name}</span>
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-bg">
            <div className={`h-full rounded-full ${tone === "primary" ? "bg-primary" : "bg-mint"}`} style={{ width: `${Math.max(6, (it.weight / max) * 100)}%` }} />
          </div>
          <span className="w-10 shrink-0 text-right text-[11px] text-text3">{pct(it.weight)}</span>
        </div>
      ))}
    </div>
  )
}

function ProfileCard({ profile, loading }: { profile?: TasteProfile; loading: boolean }) {
  if (loading || !profile) {
    return <div className="h-[220px] rounded-[24px] skeleton" />
  }
  const topHomeMood = Object.entries(profile.home_mood_counts || {}).sort((a, b) => b[1] - a[1])[0]
  return (
    <section className={`rounded-[24px] border border-border bg-card p-4 ${cardShadow}`}>
      <div className="mb-3 flex items-start gap-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-purple-light text-purple">
          <Bot size={22} strokeWidth={2.4} />
        </span>
        <div className="min-w-0">
          <div className="text-[17px] font-extrabold leading-tight">你的口味画像</div>
          <div className="mt-0.5 text-[11px] font-medium text-text3">近 {profile.window_days} 天 · 推荐官据此为你点菜</div>
        </div>
      </div>

      <p className="mb-4 text-[13px] leading-relaxed text-text2">{profile.summary}</p>

      <div className="mb-4 grid grid-cols-4 gap-2">
        <div className="rounded-2xl bg-bg px-2 py-2.5 text-center">
          <div className="text-lg font-extrabold leading-tight text-primary">{pct(profile.spicy_ratio)}</div>
          <div className="text-[10px] font-semibold text-text3">辣味占比</div>
        </div>
        <div className="rounded-2xl bg-bg px-2 py-2.5 text-center">
          <div className="text-lg font-extrabold leading-tight">{profile.window_records}</div>
          <div className="text-[10px] font-semibold text-text3">餐次</div>
        </div>
        <div className="rounded-2xl bg-bg px-2 py-2.5 text-center">
          <div className="text-lg font-extrabold leading-tight">{profile.distinct_dishes}</div>
          <div className="text-[10px] font-semibold text-text3">不同菜</div>
        </div>
        <div className="rounded-2xl bg-bg px-2 py-2.5 text-center">
          <div className="text-lg font-extrabold leading-tight">{profile.avg_cook_time > 0 ? `${Math.round(profile.avg_cook_time)}m` : "-"}</div>
          <div className="text-[10px] font-semibold text-text3">平均耗时</div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <div className="mb-1.5 text-[12px] font-bold text-text3">口味偏好</div>
          <WeightBars items={profile.taste_weights} tone="primary" />
        </div>
        <div>
          <div className="mb-1.5 text-[12px] font-bold text-text3">常吃菜系</div>
          <WeightBars items={profile.category_weights} tone="mint" />
        </div>
      </div>

      {(profile.top_dishes.length > 0 || profile.top_ingredients.length > 0 || topHomeMood) && (
        <div className="mt-4 flex flex-wrap gap-1.5">
          {profile.top_dishes.slice(0, 3).map((d) => (
            <span key={d.dish_id} className="rounded-full bg-primary-light px-2.5 py-1 text-[11px] font-semibold text-primary">🍽 常吃 {d.dish_name} ×{d.count}</span>
          ))}
          {profile.top_ingredients.slice(0, 3).map((i) => (
            <span key={i.name} className="rounded-full bg-mint-light px-2.5 py-1 text-[11px] font-semibold text-mint">🥬 {i.name}</span>
          ))}
          {topHomeMood && (
            <span className="rounded-full bg-yellow-light px-2.5 py-1 text-[11px] font-semibold text-yellow-dark">😃 常见心情 {homeMoodLabel[topHomeMood[0]] || topHomeMood[0]}</span>
          )}
          {profile.disliked_dishes.length > 0 && (
            <span className="rounded-full bg-red-light px-2.5 py-1 text-[11px] font-semibold text-red">🙅 避开 {profile.disliked_dishes.length} 道踩雷菜</span>
          )}
        </div>
      )}
    </section>
  )
}

function RecommendRow({ item, recommendationId, onEat, onReject, onDislike, disabled }: {
  item: RecommendItem
  recommendationId: string
  onDislike: () => void
  onEat: (meal: "lunch" | "dinner") => void
  onReject: () => void
  disabled: boolean
}) {
  const navigate = useNavigate()
  const { dish } = item
  const cardRef = useRef<HTMLDivElement>(null)
  useRecommendationImpression(cardRef, recommendationId, dish.id)
  return (
    <div ref={cardRef} className={`overflow-hidden rounded-[22px] border border-border bg-card ${cardShadow}`}>
      <div className="flex gap-3 p-3">
        <button onClick={() => navigate(`/dishes/${dish.id}`)} className="h-[84px] w-[84px] shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-light to-pink-light" aria-label={`查看${dish.name}`}>
          <DishImage dish={dish} className="h-full w-full" emojiSize="text-[34px]" />
        </button>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <button onClick={() => navigate(`/dishes/${dish.id}`)} className="min-w-0 text-left">
              <div className="truncate text-[16px] font-extrabold leading-tight">{dish.name}</div>
            </button>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[10px] font-semibold text-text2">
            <span className="rounded-full bg-bg px-2 py-0.5">{dish.category}</span>
            <span className="flex items-center gap-1 rounded-full bg-bg px-2 py-0.5"><Clock size={10} strokeWidth={2.5} />{dish.cook_time > 0 ? `${dish.cook_time}分钟` : "时间未填"}</span>
            <span className="rounded-full bg-bg px-2 py-0.5">{diffLabel(dish.difficulty)}</span>
            {dish.taste && <span className="rounded-full bg-pink-light px-2 py-0.5 text-pink">{dish.taste}</span>}
          </div>
          <div className="mt-1.5 flex flex-wrap gap-1">
            {item.reasons.map((r) => (
              <span key={r} className="text-[11px] text-text3">💡 {r}</span>
            ))}
          </div>
        </div>
      </div>
      <div className="grid grid-cols-[1fr_1fr_auto] gap-2 border-t border-border bg-bg/55 px-3 py-2.5">
        <button onClick={() => onEat("lunch")} disabled={disabled} className="h-9 rounded-2xl bg-primary text-[12px] font-extrabold text-white transition-all active:scale-95 disabled:opacity-50">🍳 中午吃</button>
        <button onClick={() => onEat("dinner")} disabled={disabled} className="h-9 rounded-2xl bg-mint text-[12px] font-extrabold text-white transition-all active:scale-95 disabled:opacity-50">🍲 晚上吃</button>
        <button onClick={onReject} disabled={disabled} className="flex h-9 items-center gap-1 rounded-2xl bg-card px-3 text-[12px] font-extrabold text-text3 transition-all hover:text-red active:scale-95 disabled:opacity-50" title="今天不想吃" aria-label="今天不想吃">
          <ThumbsDown size={14} strokeWidth={2.4} /> 今天不想吃
        </button>
      </div>
      <button type="button" onClick={onDislike} disabled={disabled} className="w-full py-2 text-[11px] text-text3 disabled:opacity-50">不喜欢这道，以后少推荐</button>
    </div>
  )
}

export default function Assistant() {
  const navigate = useNavigate()
  const qc = useQueryClient()

  const [meal, setMeal] = useState<MealChoice>("")
  const [mood, setMood] = useState("")
  const [tastes, setTastes] = useState<string[]>([])
  const [maxCookTime, setMaxCookTime] = useState(0)
  const [excludeText, setExcludeText] = useState("")
  const [includeText, setIncludeText] = useState("")
  const [items, setItems] = useState<RecommendItem[]>([])
  const [rejected, setRejected] = useState<number[]>([])
  const [lastSummary, setLastSummary] = useState("")
  const [recommendationId, setRecommendationId] = useState("")
  const [feedbackBusy, setFeedbackBusy] = useState(false)
  const seenRef = useRef(new Set<number>())
  const contextRef = useRef("")

  const { data: profile, isLoading: profileLoading } = useQuery({
    queryKey: ["profile", 90],
    queryFn: () => profileApi.get(90),
  })
  const { data: settings } = useQuery({ queryKey: ["settings"], queryFn: () => settingsApi.get() })
  const tasteOptions = useMemo(() => parseList(settings?.tastes, DEFAULT_TASTES), [settings])

  const buildRequest = (extraExclude: number[] = []): SmartPickRequest => {
    const context = JSON.stringify([recommendationDate(), meal, mood, tastes, maxCookTime, includeText, excludeText])
    const sameContext = contextRef.current === context
    if (!sameContext) seenRef.current.clear()
    contextRef.current = context
    const req: SmartPickRequest = { count: 4, mode: "assistant", meal_date: recommendationDate(), refresh: true, session_id: sameContext ? recommendationId || undefined : undefined }
    if (meal) req.meal_type = meal
    if (mood) req.mood = mood
    if (tastes.length > 0) req.tastes = tastes
    if (maxCookTime > 0) req.max_cook_time = maxCookTime
    const exclude = excludeText.split(/[,，、\s]+/).map((s) => s.trim()).filter(Boolean)
    if (exclude.length > 0) req.exclude_ingredients = exclude
    const include = includeText.split(/[,，、\s]+/).map((s) => s.trim()).filter(Boolean)
    if (include.length > 0) req.include_ingredients = include
    const ex = [...new Set([...rejected, ...seenRef.current, ...extraExclude])]
    if (ex.length > 0) req.exclude_dish_ids = ex
    return req
  }

  const recommendMut = useMutation({
    mutationFn: (req: SmartPickRequest) => pickApi.smart(req),
    onSuccess: (data) => {
      setItems(data.items)
      setRecommendationId(data.recommendation_id)
      data.items.forEach((item) => seenRef.current.add(item.dish.id))
      setLastSummary(data.profile_summary || "")
      qc.invalidateQueries({ queryKey: ["achievements"] })
      if (data.items.length === 0) toast.error("没有符合条件的菜，放宽一下试试")
    },
    onError: () => { contextRef.current = ""; toast.error("暂无可推荐的菜品，放宽条件再试") },
  })

  const recordMut = useMutation({
    mutationFn: (payload: { item: RecommendItem; meal: "lunch" | "dinner" }) =>
      mealPlansApi.add({ dish_id: payload.item.dish.id, meal_type: payload.meal, meal_date: recommendationDate(), recommendation_id: recommendationId || undefined }),
    onSuccess: (_res, payload) => {
      toast.success(`❤ 已加入今日${payload.meal === "lunch" ? "午餐" : "晚餐"}`)
      qc.invalidateQueries({ queryKey: ["records"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      qc.invalidateQueries({ queryKey: ["profile"] })
      for (const key of ["meal-plans", "week-plan", "shopping-list", "shopping-overview"]) void qc.invalidateQueries({ queryKey: [key] })
    },
    onError: (err: unknown) => toast.error((err instanceof Error ? err.message : "") || "记录失败"),
  })

  async function reject(item: RecommendItem, eventType: "not_today" | "dislike") {
    if (feedbackBusy || !recommendationId) return
    setFeedbackBusy(true)
    try {
      await recommendationFeedbackApi.send(recommendationId, item.dish.id, eventType)
      setRejected((prev) => [...prev, item.dish.id])
      setItems((prev) => prev.filter((it) => it.dish.id !== item.dish.id))
      if (eventType === "dislike") {
        toast((t) => <span>已减少推荐 <button className="ml-2 font-bold text-primary" onClick={() => {
          void recommendationFeedbackApi.send(recommendationId, item.dish.id, "undo_dislike").then(() => toast.dismiss(t.id)).catch(() => toast.error("撤销失败，请重试"))
        }}>撤销</button></span>)
      } else toast("这餐先不推荐这道", { icon: "👌" })
    } catch (error) { toast.error(error instanceof Error ? error.message : "反馈保存失败，请重试") }
    finally { setFeedbackBusy(false) }
  }

  function toggleTaste(t: string) {
    setTastes((prev) => (prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t]))
  }

  const busy = recommendMut.isPending || recordMut.isPending || feedbackBusy

  return (
    <div className="animate-fadeUp flex h-dvh flex-col overflow-hidden">
      <PageHeader
        title="AI 推荐官"
        subtitle="按你的口味画像点菜，也能聊着问做法"
        icon={Sparkles}
        onBack={() => navigate(-1)}
      />

      <main className="app-scroll min-h-0 flex-1 overflow-y-auto px-5 py-4 pb-10">
        <div className="mx-auto max-w-[640px] space-y-4">
          <ProfileCard profile={profile} loading={profileLoading} />

          <section className={`rounded-[24px] border border-border bg-card p-4 ${cardShadow}`}>
            <div className="mb-3 flex items-center justify-between">
              <div>
                <div className="text-[15px] font-extrabold">今天想怎么吃</div>
                <div className="text-[11px] font-medium text-text3">条件会和口味画像一起打分</div>
              </div>
              <button
                onClick={() => recommendMut.mutate(buildRequest())}
                disabled={busy}
                className="flex h-10 items-center gap-1.5 rounded-full bg-primary px-4 text-[13px] font-extrabold text-white transition-all hover:bg-primary-dark active:scale-95 disabled:opacity-50"
              >
                <RefreshCw size={15} strokeWidth={2.5} className={recommendMut.isPending ? "animate-spin" : ""} />
                {items.length > 0 ? "再来一批" : "给我推荐"}
              </button>
            </div>

            <div className="mb-3 flex gap-2">
              {mealOptions.map((m) => (
                <button key={m.key} onClick={() => setMeal(m.key)} className={`flex-1 rounded-2xl border px-2 py-2 text-[12px] font-bold transition-all active:scale-95 ${meal === m.key ? "border-primary bg-primary-light text-primary" : "border-border bg-bg text-text2"}`}>
                  {m.label}
                </button>
              ))}
            </div>

            <div className="mb-3 flex gap-2 overflow-x-auto pb-1 scrollbar-none">
              {moodOptions.map((m) => {
                const Icon = m.Icon
                const active = mood === m.key
                return (
                  <button key={m.key} onClick={() => setMood(m.key)} className={`flex shrink-0 items-center gap-1.5 rounded-2xl border px-3 py-2 text-[12px] font-bold transition-all active:scale-95 ${active ? "border-primary bg-primary-light text-primary" : "border-border bg-card text-text2"}`}>
                    <Icon size={15} strokeWidth={2.4} />
                    {m.label}
                  </button>
                )
              })}
            </div>

            <div className="mb-3">
              <div className="mb-1.5 text-[11px] font-bold text-text3">想要的口味（可多选）</div>
              <div className="flex flex-wrap gap-1.5">
                {tasteOptions.map((t) => (
                  <button key={t} onClick={() => toggleTaste(t)} className={`rounded-full border px-3 py-1 text-[12px] font-semibold transition-all active:scale-95 ${tastes.includes(t) ? "border-pink bg-pink text-white" : "border-border bg-bg text-text2"}`}>
                    {t}
                  </button>
                ))}
              </div>
            </div>

            <div className="mb-3">
              <div className="mb-1.5 text-[11px] font-bold text-text3">烹饪时间</div>
              <div className="flex gap-1.5">
                {cookTimeOptions.map((o) => (
                  <button key={o.value} onClick={() => setMaxCookTime(o.value)} className={`flex-1 rounded-full border px-2 py-1.5 text-[11px] font-semibold transition-all active:scale-95 ${maxCookTime === o.value ? "border-mint bg-mint-light text-mint" : "border-border bg-bg text-text2"}`}>
                    {o.label}
                  </button>
                ))}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2">
              <label className="block">
                <span className="mb-1 block text-[11px] font-bold text-text3">想用的食材</span>
                <input value={includeText} onChange={(e) => setIncludeText(e.target.value)} placeholder="如：牛肉 青椒" className="h-10 w-full rounded-2xl border-[1.5px] border-border bg-bg px-3 text-[13px] outline-none transition-all focus:border-primary" />
              </label>
              <label className="block">
                <span className="mb-1 block text-[11px] font-bold text-text3">忌口 / 不想吃</span>
                <input value={excludeText} onChange={(e) => setExcludeText(e.target.value)} placeholder="如：香菜 内脏" className="h-10 w-full rounded-2xl border-[1.5px] border-border bg-bg px-3 text-[13px] outline-none transition-all focus:border-primary" />
              </label>
            </div>
          </section>

          {items.length > 0 && (
            <section>
              <div className="mb-3 flex items-center justify-between">
                <div className="text-[15px] font-extrabold">为你挑了 {items.length} 道</div>
                {lastSummary && <div className="max-w-[60%] truncate text-[11px] text-text3">{lastSummary}</div>}
              </div>
              <div className="space-y-3">
                {items.map((it) => (
                  <RecommendRow
                    key={it.dish.id}
                    item={it}
                    recommendationId={recommendationId}
                    disabled={busy}
                    onEat={(m) => recordMut.mutate({ item: it, meal: m })}
                    onReject={() => void reject(it, "not_today")}
                    onDislike={() => void reject(it, "dislike")}
                  />
                ))}
              </div>
            </section>
          )}

          <section className={`overflow-hidden rounded-[24px] border border-border bg-card ${cardShadow}`}>
            <div className="flex items-center gap-3 border-b border-border px-4 py-3">
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-primary-light text-primary">
                <Bot size={22} strokeWidth={2.4} />
              </span>
              <div className="min-w-0">
                <div className="text-[15px] font-extrabold leading-tight">和食谱小助手聊聊</div>
                <div className="mt-0.5 text-[11px] font-medium text-text3">问做法、排一周菜单、按冰箱食材配菜</div>
              </div>
            </div>
            <div className="px-4 py-6">
              <p className="text-sm leading-relaxed text-text2">告诉助手你的口味、手边食材或想花的时间，直接获得适合自己的建议。两端共用每日提问次数，可在对话页查看剩余额度。</p>
              <button onClick={() => navigate("/assistant/chat")} className="mt-4 min-h-11 rounded-full bg-primary px-5 text-sm font-semibold text-white active:scale-95">开始对话</button>
            </div>
          </section>
        </div>
      </main>
    </div>
  )
}
