import { useState, useEffect, useRef, useCallback, useMemo } from "react"
import { createPortal } from "react-dom"
import { useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { pickApi, mealPlansApi, dayRatingApi, recommendationFeedbackApi } from "@/api"
import type { RecommendItem } from "@/types"
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
import { recommendationDate } from "@/lib/recommendation"
import { useRecommendationImpression } from "@/lib/use-recommendation-impression"

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
export default function Home() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [todayKey, setTodayKey] = useState(recommendationDate)
  const isAdmin = useAuthStore((s) => s.user?.role === "admin")
  const appName = useAppInfoStore((s) => s.appName)
  const palette = useAppearanceStore((s) => s.palette)
  const displayMode = useAppearanceStore((s) => s.mode)
  const systemDark = useAppearanceStore((s) => s.systemDark)

  const [recItems, setRecItems] = useState<RecommendItem[]>([])
  const [recIdx, setRecIdx] = useState(0)
  const [recQuote, setRecQuote] = useState("")
  const [recMeal, setRecMeal] = useState<"lunch" | "dinner" | null>(null)
  const [recommendationId, setRecommendationId] = useState("")
  const [profileSummary, setProfileSummary] = useState("")
  const [selectedMood, setSelectedMood] = useState<string | null>(null)
  const [spinning, setSpinning] = useState(false)
  const [changing, setChanging] = useState(false)
  const seenRef = useRef(new Set<number>())
  const appliedPickRef = useRef("")
  const wheelDegRef = useRef(0)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const wheelRef = useRef<HTMLDivElement>(null)
  const confettiRef = useRef<HTMLDivElement>(null)
  const recCardRef = useRef<HTMLDivElement>(null)

  const { data: todayRating, isLoading: ratingLoading } = useQuery({
    queryKey: ["day-rating", todayKey], queryFn: () => dayRatingApi.get(todayKey),
  })
  const activeMood = selectedMood ?? todayRating?.home_mood ?? ""
  const { data: todayPlans = [] } = useQuery({
    queryKey: ["meal-plans", todayKey], queryFn: () => mealPlansApi.list(todayKey, todayKey),
  })
  const { data: initialPick, error: loadError, refetch: retryLoad, isFetching: initialLoading } = useQuery({
    queryKey: ["pick", "smart", "initial", todayKey, recMeal, activeMood],
    queryFn: () => pickApi.smart({ count: 3, mode: "home_auto", meal_date: todayKey, meal_type: recMeal || undefined, mood: activeMood || undefined }),
    enabled: !ratingLoading, staleTime: 0, retry: 0,
  })
  const { data: wheelData } = useQuery({
    queryKey: ["pick", "wheel", todayKey, recMeal, activeMood],
    queryFn: () => pickApi.smart({ count: 10, mode: "wheel", meal_date: todayKey, meal_type: recMeal || undefined, mood: activeMood || undefined }),
    enabled: !ratingLoading,
  })
  useEffect(() => {
    const updateDate = () => {
      const next = recommendationDate()
      if (next !== todayKey) { setTodayKey(next); setSelectedMood(null); seenRef.current.clear() }
    }
    const refresh = () => { updateDate(); void qc.invalidateQueries({ queryKey: ["pick"] }) }
    const interval = window.setInterval(updateDate, 30_000)
    window.addEventListener("focus", refresh)
    return () => { window.clearInterval(interval); window.removeEventListener("focus", refresh) }
  }, [qc, todayKey])
  useEffect(() => {
    if (!initialPick || appliedPickRef.current === initialPick.recommendation_id) return
    appliedPickRef.current = initialPick.recommendation_id
    seenRef.current.clear()
    // Replace stale results after date, mood or dietary preference changes.
    setRecItems(initialPick.items || [])
    setRecIdx(0)
    setRecommendationId(initialPick.recommendation_id)
    setRecQuote(initialPick.quote || "")
    setProfileSummary(initialPick.profile_summary || "")
  }, [initialPick])
  const wheelDishes = useMemo(() => wheelData?.items.map((item) => item.dish) || [], [wheelData])
  const currentItem = recItems[recIdx] ?? null
  const currentRec = currentItem?.dish || null
  useRecommendationImpression(recCardRef, recommendationId, currentItem?.dish.id)
  useEffect(() => { if (currentItem) seenRef.current.add(currentItem.dish.id) }, [currentItem])
  const { data: moodPick } = useQuery({
    queryKey: ["pick", "mood", todayKey, activeMood],
    queryFn: () => pickApi.mood(activeMood), enabled: !!activeMood,
  })
  function chooseMeal(meal: "lunch" | "dinner") {
    seenRef.current.clear()
    setRecMeal(meal)
    requestAnimationFrame(() => scrollToElement(recCardRef.current, { block: "center" }))
  }
  const recordMut = useMutation({
    mutationFn: (data: { dish_id: number; meal_type: string; meal_date: string; recommendation_id?: string }) => mealPlansApi.add(data),
    onSuccess: () => {
      toast.success("❤ 已加入菜单，吃过后再确认记录")
      launchConfetti(confettiRef.current)
      for (const key of ["meal-plans", "week-plan", "shopping-list", "shopping-overview", "health-report", "achievements"]) void qc.invalidateQueries({ queryKey: [key] })
    },
    onError: (err: unknown) => toast.error((err instanceof Error ? err.message : "") || "加入菜单失败"),
  })
  const homeMoodMut = useMutation({
    mutationFn: (mood: string) => dayRatingApi.updateHomeMood({ meal_date: todayKey, home_mood: mood }),
    onSuccess: () => {
      for (const key of ["day-rating", "day-ratings", "photo-wall", "achievements", "pick"]) void qc.invalidateQueries({ queryKey: [key] })
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
        const item = wheelData?.items[pickIdx]
        setRecItems(item ? [item] : [])
        setRecIdx(0)
        setRecommendationId(wheelData?.recommendation_id || "")
        setRecQuote("")
        toast.success(`🎯 转到了：${pick.name}！`)
        requestAnimationFrame(() => scrollToElement(recCardRef.current, { block: "center" }))
      },
    })
  }

  function handlePickMeal(mealType: "lunch" | "dinner") {
    if (!currentRec) return
    const today = todayKey
    const alreadyPicked = todayPlans.some(
      (r) => r.dish_id === currentRec.id && r.meal_type === mealType && r.meal_date === today
    )
    if (alreadyPicked) {
      toast.error(`${currentRec.name} 已在今日${mealType === "lunch" ? "午餐" : "晚餐"}菜单中啦~`)
      return
    }
    recordMut.mutate({
      dish_id: currentRec.id,
      meal_type: mealType,
      meal_date: today,
      recommendation_id: currentItem ? recommendationId : undefined,
    })
  }

  function handleMoodPick(mood: string) {
    setSelectedMood(mood)
    homeMoodMut.mutate(mood)
    const label = moods.find(m => m.key === mood)?.label || mood
    toast.success(`心情：${label}，已保存并调整推荐~`)
  }

  // A skip applies to this recommendation session, not long-term taste.
  async function changeRecommend() {
    if (changing) return
    setChanging(true)
    try {
      if (currentItem && recommendationId) await recommendationFeedbackApi.send(recommendationId, currentItem.dish.id, "skip")
      if (currentRec) seenRef.current.add(currentRec.id)
        if (currentItem && recIdx < recItems.length - 1) { setRecIdx(recIdx + 1); return }
      const data = await pickApi.smart({
        meal_type: recMeal || undefined, meal_date: todayKey, mood: activeMood || undefined,
        count: 3, exclude_dish_ids: [...seenRef.current], session_id: recommendationId || undefined, mode: "home_change", refresh: true,
      })
      if (!data.items?.length) { toast.error("符合条件的菜已经看完，可以调整条件再试"); return }
      setRecItems(data.items)
      setRecIdx(0)
      setRecommendationId(data.recommendation_id)
      setProfileSummary(data.profile_summary || profileSummary)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "暂时无法换菜，请重试")
    } finally { setChanging(false) }
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
            onClick={() => chooseMeal("lunch")}
            disabled={initialLoading}
            aria-pressed={recMeal === "lunch"}
            className="meal-entry relative p-5 overflow-hidden transition-all active:scale-97 border-[1.5px] text-left disabled:opacity-60"
          >
            <span className="text-[28px] block mb-2">🍳</span>
            <div className="text-[15px] font-bold mb-0.5">中午吃点好的</div>
            <div className="text-xs text-text2">{initialLoading && recMeal === null ? "推荐中..." : "午餐推荐 →"}</div>
          </button>
          <button
            onClick={() => chooseMeal("dinner")}
            disabled={initialLoading}
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
              className={`flex-shrink-0 p-3 px-4 rounded-2xl border-2 text-center transition-all active:scale-95 min-w-[80px] ${activeMood === m.key ? "border-primary bg-primary-light" : "border-border bg-card"}`}
            >
              <span className="text-[28px] block mb-1">{m.emoji}</span>
              <span className={`text-[11px] font-medium ${activeMood === m.key ? "text-primary" : "text-text2"}`}>{m.label}</span>
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
