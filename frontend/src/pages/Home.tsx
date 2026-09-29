import { useState, useEffect, useRef, useCallback, useMemo } from "react"
import { createPortal } from "react-dom"
import { useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { pickApi, dishesApi, recordsApi, dayRatingApi, behaviorApi } from "@/api"
import type { Dish, RecommendItem } from "@/types"
import DishImage from "@/components/DishImage"
import DishCard from "@/components/DishCard"
import SectionHeader from "@/components/SectionHeader"
import PageHeader, { HeaderIconButton } from "@/components/PageHeader"
import RequestState from "@/components/RequestState"
import { cardShadow } from "@/components/Card"
import { useAuthStore } from "@/store/useAuthStore"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import { launchConfetti } from "@/lib/confetti"
import { gsap, motionDuration, scrollToElement, useGSAP } from "@/lib/gsap"
import toast from "react-hot-toast"
import { Home as HomeIcon, Settings, Sparkles } from "lucide-react"
import HealthSummaryCard from "@/components/HealthSummaryCard"
import ThemePicker from "@/components/ThemePicker"
import { useAppearanceStore } from "@/store/useAppearanceStore"
import { getDishImageUrl } from "@/lib/dish-image"

const moods = [
  { key: "happy", emoji: "😊", label: "开心" },
  { key: "tired", emoji: "😫", label: "疲惫" },
  { key: "lazy", emoji: "😌", label: "想偷懒" },
  { key: "spicy", emoji: "🤤", label: "想吃辣" },
  { key: "healthy", emoji: "🌿", label: "想养生" },
]

function getGreeting(): string {
  const h = new Date().getHours()
  if (h < 6) return "夜深了~"
  if (h < 11) return "早上好 ☀️"
  if (h < 14) return "中午好 🍳"
  if (h < 18) return "下午好 ☀️"
  return "晚上好 🌙"
}

function diffLabel(d: string) { return d === "easy" ? "简单" : d === "medium" ? "中等" : "困难" }
function dateKey(d = new Date()) {
  return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,"0")}-${String(d.getDate()).padStart(2,"0")}`
}

export default function Home() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const todayKey = dateKey()
  const isAdmin = useAuthStore((s) => s.user?.role === "admin")
  const appName = useAppInfoStore((s) => s.appName)
  const palette = useAppearanceStore((s) => s.palette)
  const displayMode = useAppearanceStore((s) => s.mode)
  const systemDark = useAppearanceStore((s) => s.systemDark)

  const { data: dishesData, error: loadError, refetch: retryLoad } = useQuery({
    queryKey: ["dishes", "enabled"],
    queryFn: () => dishesApi.list({ enabled: "true", pageSize: "20", sort: "sort_order", order: "asc" }),
  })
  const { data: wheelData } = useQuery({
    queryKey: ["dishes", "wheel"],
    queryFn: () => dishesApi.list({ enabled: "true", pageSize: "12", sort: "random" }),
  })
  const { data: recordsData } = useQuery({
    queryKey: ["records", "date", todayKey],
    queryFn: () => recordsApi.forDates(todayKey, todayKey),
  })
  const { data: todayRating } = useQuery({
    queryKey: ["day-rating", todayKey],
    queryFn: () => dayRatingApi.get(todayKey),
  })
  // 首屏推荐：按口味画像挑 3 道，供“今日推荐 / 换一个”使用
  const { data: initialPick } = useQuery({
    queryKey: ["pick", "smart", "initial"],
    queryFn: () => pickApi.smart({ count: 3, mode: "home_auto" }),
    staleTime: Infinity,
    retry: 0,
  })

  const [pickedRec, setPickedRec] = useState<Dish | null>(null)
  const [recItems, setRecItems] = useState<RecommendItem[]>([])
  const [recIdx, setRecIdx] = useState(0)
  const [recQuote, setRecQuote] = useState<string>("")
  const [recMeal, setRecMeal] = useState<"lunch" | "dinner" | null>(null)
  const [profileSummary, setProfileSummary] = useState("")
  const [selectedMood, setSelectedMood] = useState<string | null>(todayRating?.home_mood || null)
  const [spinning, setSpinning] = useState(false)
  const [changing, setChanging] = useState(false)
  const wheelDegRef = useRef(0)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const wheelRef = useRef<HTMLDivElement>(null)
  const confettiRef = useRef<HTMLDivElement>(null)
  const recCardRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!initialPick?.items?.length) return
    // Seed the recommendation interaction state once the query result is available.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setRecItems((prev) => (prev.length > 0 ? prev : initialPick.items))
    setProfileSummary((prev) => prev || initialPick.profile_summary || "")
  }, [initialPick])

  const dishes = dishesData?.items || []
  const wheelDishes = useMemo(() => wheelData?.items || [], [wheelData])
  const todayRecords = recordsData?.items || []
  const currentItem: RecommendItem | null = pickedRec ? null : (recItems[recIdx] ?? null)
  const currentRec: Dish | null = pickedRec || currentItem?.dish || dishes[0] || null

  const { data: moodPick } = useQuery({
    queryKey: ["pick", "mood", selectedMood],
    queryFn: () => pickApi.mood(selectedMood!),
    enabled: !!selectedMood,
  })

  useEffect(() => {
    if (moodPick?.dishes?.length) {
      qc.invalidateQueries({ queryKey: ["achievements"] })
    }
  }, [moodPick, qc])

  // 午/晚餐推荐：走智能推荐引擎，带推荐理由，结果写入当前推荐卡并滚动过去
  const pickMealMut = useMutation({
    mutationFn: (meal: "lunch" | "dinner") => pickApi.smart({ meal_type: meal, count: 3, mood: selectedMood || undefined, mode: "home_meal" }),
    onSuccess: (data, meal) => {
      if (!data?.items?.length) {
        toast.error("暂无可推荐的菜品")
        return
      }
      setPickedRec(null)
      setRecItems(data.items)
      setRecIdx(0)
      setRecQuote(data.quote || "")
      setRecMeal(meal)
      setProfileSummary(data.profile_summary || "")
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success(`${meal === "lunch" ? "🍳 午餐" : "🍲 晚餐"}推荐：${data.items[0].dish.name}`)
      requestAnimationFrame(() => scrollToElement(recCardRef.current, { block: "center" }))
    },
    onError: () => toast.error("推荐失败，稍后再试"),
  })

  const recordMut = useMutation({
    mutationFn: (data: { dish_id: number; dish_name: string; meal_type: string; meal_date: string }) =>
      recordsApi.create(data),
    onSuccess: (_res, variables) => {
      toast.success("❤ 已记录！")
      launchConfetti(confettiRef.current)
      qc.invalidateQueries({ queryKey: ["records"] })
      qc.invalidateQueries({ queryKey: ["health-report"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      // 采纳推荐 → 行为事件，供推荐分析使用
      void behaviorApi.log({
        event_type: "accept",
        dish_id: variables.dish_id,
        dish_name: variables.dish_name,
        meta: { from: pickedRec ? "home_pick" : "home_recommend", meal_type: variables.meal_type },
      })
    },
    onError: (err: unknown) => {
      const msg = (err instanceof Error ? err.message : "") || "记录失败"
      toast.error(msg)
    },
  })

  const homeMoodMut = useMutation({
    mutationFn: (mood: string) => dayRatingApi.updateHomeMood({ meal_date: todayKey, home_mood: mood }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["day-rating", todayKey] })
      qc.invalidateQueries({ queryKey: ["day-ratings"] })
      qc.invalidateQueries({ queryKey: ["photo-wall"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
    },
    onError: () => toast.error("心情保存失败"),
  })

  const drawWheel = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas || wheelDishes.length === 0) return
    const ctx = canvas.getContext("2d")
    if (!ctx) return
    const cx = 106, cy = 106, r = 105
    const style = getComputedStyle(document.documentElement)
    const colors = ["--color-primary-light", "--color-inset", "--color-card"].map((token) => style.getPropertyValue(token).trim())
    const n = wheelDishes.length
    const angle = (2 * Math.PI) / n
    ctx.clearRect(0, 0, 212, 212)
    for (let i = 0; i < n; i++) {
      ctx.beginPath()
      ctx.moveTo(cx, cy)
      ctx.arc(cx, cy, r, i * angle - Math.PI / 2, (i + 1) * angle - Math.PI / 2)
      ctx.closePath()
      ctx.fillStyle = colors[i % colors.length]
      ctx.fill()
      ctx.save()
      ctx.translate(cx, cy)
      ctx.rotate(i * angle + angle / 2 - Math.PI / 2)
      ctx.fillStyle = style.getPropertyValue("--color-text").trim()
      ctx.font = "bold 11px -apple-system, sans-serif"
      ctx.textAlign = "center"
      ctx.fillText(wheelDishes[i].name, r * 0.6, 4)
      ctx.restore()
    }
  }, [wheelDishes])

  useEffect(() => {
    drawWheel()
  }, [drawWheel, palette, displayMode, systemDark])

  useGSAP(() => {
    gsap.set(wheelRef.current, { rotation: wheelDegRef.current, transformOrigin: "50% 50%" })
  }, { scope: wheelRef })

  function spinWheel() {
    if (spinning || wheelDishes.length === 0) return
    setSpinning(true)
    const pickIdx = Math.floor(Math.random() * wheelDishes.length)
    const seg = 360 / wheelDishes.length
    const targetWithin = (360 - (pickIdx * seg + seg / 2)) % 360
    const base = wheelDegRef.current - (wheelDegRef.current % 360)
    const nextRotation = base + 360 * 5 + targetWithin
    wheelDegRef.current = nextRotation

    gsap.to(wheelRef.current, {
      rotation: nextRotation,
      duration: motionDuration(3),
      ease: "power4.out",
      overwrite: true,
      onComplete: () => {
        setSpinning(false)
        const pick = wheelDishes[pickIdx]
        setPickedRec(pick)
        setRecQuote("")
        setRecMeal(null)
        toast.success(`🎯 转到了：${pick.name}！`)
        requestAnimationFrame(() => scrollToElement(recCardRef.current, { block: "center" }))
      },
    })
  }

  function handlePickMeal(mealType: "lunch" | "dinner") {
    if (!currentRec) return
    const today = todayKey
    const alreadyPicked = todayRecords.some(
      (r) => r.dish_id === currentRec.id && r.meal_type === mealType && r.meal_date === today
    )
    if (alreadyPicked) {
      toast.error(`${currentRec.name} 已在今日${mealType === "lunch" ? "午餐" : "晚餐"}中记录过啦~`)
      return
    }
    recordMut.mutate({
      dish_id: currentRec.id,
      dish_name: currentRec.name,
      meal_type: mealType,
      meal_date: today,
    })
  }

  function handleMoodPick(mood: string) {
    setSelectedMood(mood)
    homeMoodMut.mutate(mood)
    const label = moods.find(m => m.key === mood)?.label || mood
    toast.success(`心情：${label}，已保存并调整推荐~`)
  }

  // 换一个：先在本批推荐里翻页，翻完了按已看过的菜排除后再要一批；被跳过的菜记一条 reject 事件
  async function changeRecommend() {
    if (changing) return
    if (currentItem) {
      void behaviorApi.log({
        event_type: "reject",
        dish_id: currentItem.dish.id,
        dish_name: currentItem.dish.name,
        meta: { from: "home_recommend", meal_type: recMeal || "" },
      })
    }
    setPickedRec(null)
    if (recItems.length > 0 && recIdx < recItems.length - 1) {
      setRecIdx(recIdx + 1)
      return
    }
    setChanging(true)
    try {
      const seen = recItems.map((it) => it.dish.id)
      const data = await pickApi.smart({
        meal_type: recMeal || undefined,
        mood: selectedMood || undefined,
        count: 3,
        exclude_dish_ids: seen,
        mode: "home_change",
      })
      if (!data?.items?.length) {
        toast.error("没有更多推荐了")
        return
      }
      setRecItems(data.items)
      setRecIdx(0)
      setProfileSummary(data.profile_summary || profileSummary)
    } catch {
      // 回退：在已加载的菜单里顺序换
      if (dishes.length === 0) return
      const idx = currentRec ? dishes.findIndex((d) => d.id === currentRec.id) : -1
      setPickedRec(dishes[(idx + 1) % dishes.length])
    } finally {
      setChanging(false)
    }
  }

  const moodDishes = moodPick?.dishes

  return (
    <div className="animate-fadeUp">
      {createPortal(<div ref={confettiRef} className="fixed inset-0 z-50 pointer-events-none overflow-hidden" />, document.body)}

      <PageHeader
        title={<span>{appName.slice(0, Math.max(1, appName.length - 4))}<span className="text-primary">{appName.slice(Math.max(1, appName.length - 4))}</span></span>}
        subtitle="今天也好好吃饭"
        icon={HomeIcon}
        actions={
          <>
           <ThemePicker />
           <HeaderIconButton onClick={() => navigate(isAdmin ? "/admin/dashboard" : "/me/preferences")} aria-label={isAdmin ? "管理设置" : "饮食偏好"}>
            <Settings size={18} strokeWidth={2.3} />
          </HeaderIconButton>
          </>
        }
      />

      <div className="home-content px-5 max-w-[640px] mx-auto">
        {loadError && <RequestState error={loadError} onRetry={() => { void retryLoad() }} compact />}
        <div className="home-greeting">
          <div className="text-sm text-text2 mb-1">{getGreeting()}</div>
          <div className="text-[26px] font-extrabold tracking-tight leading-tight">今天<em className="not-italic text-primary">想吃什么</em>？</div>
        </div>

        <HealthSummaryCard />

        {/* 午/晚餐入口：点击真正触发推荐并同步到下方推荐卡 */}
        <div className="grid grid-cols-2 gap-3 mb-6">
          <button
            onClick={() => pickMealMut.mutate("lunch")}
            disabled={pickMealMut.isPending}
            aria-pressed={recMeal === "lunch"}
            className="meal-entry relative p-5 overflow-hidden transition-all active:scale-97 border-[1.5px] text-left disabled:opacity-60"
          >
            <span className="text-[28px] block mb-2">🍳</span>
            <div className="text-[15px] font-bold mb-0.5">中午吃点好的</div>
            <div className="text-xs text-text2">{pickMealMut.isPending && recMeal === null ? "推荐中..." : "午餐推荐 →"}</div>
          </button>
          <button
            onClick={() => pickMealMut.mutate("dinner")}
            disabled={pickMealMut.isPending}
            aria-pressed={recMeal === "dinner"}
            className="meal-entry relative p-5 overflow-hidden transition-all active:scale-97 border-[1.5px] text-left disabled:opacity-60"
          >
            <span className="text-[28px] block mb-2">🍲</span>
            <div className="text-[15px] font-bold mb-0.5">晚上吃点温暖的</div>
            <div className="text-xs text-text2">晚餐推荐 →</div>
          </button>
        </div>

        {/* 今日推荐卡：提到首屏，紧随入口 */}
        {currentRec && (
          <section ref={recCardRef} className="home-recommendation" aria-label="今日推荐">
            <div className="home-recommendation__heading">
              <div><h2>今日推荐</h2><p>{recMeal === "lunch" ? "给午餐多一点期待" : recMeal === "dinner" ? "为晚餐留一点温暖" : "把今天的好心情，放进这一餐"}</p></div>
                <button onClick={() => void changeRecommend()} disabled={changing} className="shrink-0 min-h-11 text-sm font-semibold text-primary bg-primary-light px-3 py-1.5 rounded-full transition-all disabled:opacity-50">
                  {changing ? "挑选中..." : "🔄 换一个"}
                </button>
            </div>
              {getDishImageUrl(currentRec) && <button type="button" onClick={() => navigate(`/dishes/${currentRec.id}`)} aria-label={`查看${currentRec.name}做法`} className="home-recommendation__image">
                <DishImage dish={currentRec} className="w-full h-full" emojiSize="text-[64px]" />
              </button>}
              <div className="recipe-ticket">
                <div className="recipe-ticket__eyebrow">{currentItem ? "为你挑选 · 按你的口味推荐" : "今日好味 · 一起好好吃饭"}</div>
                <button type="button" onClick={() => navigate(`/dishes/${currentRec.id}`)} className="recipe-ticket__title">{currentRec.name}</button>
                <div className="recipe-ticket__meta">
                  <span>{currentRec.category}</span><span>{currentRec.cook_time} 分钟</span><span>{diffLabel(currentRec.difficulty)}</span>
                  {currentRec.taste && <span>{currentRec.taste}</span>}
                </div>
                {currentItem && currentItem.reasons.length > 0 && (
                  <div className="flex gap-1.5 flex-wrap mt-3">
                    {currentItem.reasons.map((reason) => (
                      <span key={reason} className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-yellow-light text-yellow-dark border border-yellow/30">💡 {reason}</span>
                    ))}
                  </div>
                )}
              </div>
            <div className="recommend-actions">
              <button type="button" disabled={recordMut.isPending} onClick={() => handlePickMeal("lunch")} className="recommend-actions__primary transition-transform active:scale-96">🍳 中午吃这个</button>
              <button type="button" disabled={recordMut.isPending} onClick={() => handlePickMeal("dinner")} className="recommend-actions__secondary transition-transform active:scale-96">🍲 晚上吃这个</button>
            </div>
            {recItems.length > 1 && currentItem && (
              <div className="text-center text-[11px] text-text3 mt-4">第 {recIdx + 1} / {recItems.length} 道 · 换一个会从下一道开始</div>
            )}
            {/* 今日推荐语 */}
            {recQuote && (
              <div className="text-center text-[13px] text-text2 mt-3 px-4 leading-relaxed">{recQuote}</div>
            )}
          </section>
        )}

        {/* AI 推荐官入口 */}
        <button onClick={() => navigate("/assistant")} className={`w-full bg-card rounded-2xl p-4 ${cardShadow} border border-primary/15 flex items-center gap-3.5 transition-all active:scale-98 mb-6`}>
          <div className="w-12 h-12 rounded-[10px] bg-gradient-to-br from-purple-light to-primary-light flex items-center justify-center text-primary flex-shrink-0">
            <Sparkles size={24} strokeWidth={2.4} />
          </div>
          <div className="flex-1 min-w-0 text-left">
            <div className="text-[15px] font-bold mb-0.5">🤖 AI 推荐官</div>
            <div className="text-xs text-text2 truncate">{profileSummary || "根据你的口味画像推荐，还能聊着点菜"}</div>
          </div>
          <span className="text-text3 text-sm">›</span>
        </button>

        {/* 更多玩法（下移） */}
        <div className="text-[13px] font-semibold text-text3 mb-3 flex items-center gap-2">
          <span className="flex-1 h-px bg-border" /> 更多玩法 <span className="flex-1 h-px bg-border" />
        </div>

        <section className="home-more-panel" aria-label="今天的心情">
        <SectionHeader title="😃 今天的心情" />
        <div className="flex gap-2.5 overflow-x-auto pb-3 mb-5 scrollbar-none">
          {moods.map((m) => (
            <button
              key={m.key}
              onClick={() => handleMoodPick(m.key)}
              className={`flex-shrink-0 p-3 px-4 rounded-2xl border-2 text-center transition-all active:scale-95 min-w-[80px] ${selectedMood === m.key ? "border-primary bg-primary-light" : "border-border bg-card"}`}
            >
              <span className="text-[28px] block mb-1">{m.emoji}</span>
              <span className={`text-[11px] font-medium ${selectedMood === m.key ? "text-primary" : "text-text2"}`}>{m.label}</span>
            </button>
          ))}
        </div>

        {moodDishes && moodDishes.length > 0 && (
          <div className="mb-6">
            <div className="text-base font-bold mb-3">心情推荐</div>
            <div className="grid grid-cols-2 gap-3">
              {moodDishes.slice(0, 4).map((d) => (
                <DishCard key={d.id} dish={d} onClick={() => navigate(`/dishes/${d.id}`)} />
              ))}
            </div>
          </div>
        )}
        </section>

        <section className="home-more-panel" aria-label="转一转">
        <SectionHeader title="🎰 转一转" />
        <div className="relative w-[220px] h-[220px] mx-auto mb-6">
          <div className="absolute -top-3 left-1/2 -translate-x-1/2 w-0 h-0 border-l-[12px] border-l-transparent border-r-[12px] border-r-transparent border-t-[20px] border-t-primary z-2 drop-shadow-sm" />
          <div
            ref={wheelRef}
            className="w-[220px] h-[220px] rounded-full overflow-hidden border-4 border-primary bg-card relative"
          >
            <canvas ref={canvasRef} width={212} height={212} className="w-full h-full" />
          </div>
          <button onClick={spinWheel} disabled={spinning} className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-14 h-14 rounded-full bg-card shadow-md z-2 flex items-center justify-center text-sm font-bold text-primary active:scale-90 transition-transform disabled:opacity-70">{spinning ? "🎰" : "转"}</button>
        </div>
        </section>

        <button onClick={() => navigate("/tomorrow")} className={`w-full bg-card rounded-2xl p-4 ${cardShadow} border border-border flex items-center gap-3.5 transition-all active:scale-98`}>
          <div className="w-12 h-12 rounded-[10px] bg-gradient-to-br from-yellow-light to-primary-light flex items-center justify-center text-2xl flex-shrink-0">🌙</div>
          <div className="flex-1 min-w-0 text-left">
            <div className="text-[15px] font-bold mb-0.5">🌙 明天吃什么？</div>
            <div className="text-xs text-text2">提前规划明天的菜单，不用再临时纠结</div>
          </div>
          <span className="text-text3 text-sm">›</span>
        </button>
      </div>
    </div>
  )
}
