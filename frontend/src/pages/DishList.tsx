import { useState, useMemo, useRef, useEffect, useCallback } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { useQuery, useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { dishesApi, favoritesApi, settingsApi, recordsApi } from "@/api"
import { asArray } from "@/lib/utils"
import { gsap, scrollToElement } from "@/lib/gsap"
import AnimatedBottomSheet from "@/components/AnimatedBottomSheet"
import DishImage from "@/components/DishImage"
import { isImageUrl } from "@/lib/dish-image"
import type { Dish, MealRecord } from "@/types"
import PageHeader, { HeaderIconButton } from "@/components/PageHeader"
import { useAuthStore } from "@/store/useAuthStore"
import toast from "react-hot-toast"
import RequestState from "@/components/RequestState"
import { Settings, UtensilsCrossed, Minus, Plus, ChefHat, X, Trash2 } from "lucide-react"

const DEFAULT_CATEGORIES = ["川菜", "湘菜", "贵州菜", "云南菜", "粤菜"]
const DEFAULT_TASTES = ["辣", "麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "酱香"]

function parseList(raw: unknown, fallback: string[]): string[] {
  const arr = asArray<string>(raw).filter((x) => typeof x === "string")
  return arr.length > 0 ? arr : fallback
}

function diffLabel(d: string) { return d === "easy" ? "简单" : d === "medium" ? "中等" : "困难" }
function diffColor(d: string) { return d === "easy" ? "text-mint" : d === "medium" ? "text-yellow" : "text-primary" }
function dateKey(d = new Date()) {
  return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,"0")}-${String(d.getDate()).padStart(2,"0")}`
}

const PAGE_SIZE = 30
const RECIPE_SCOPES = [
  { value: "", label: "全部菜谱" },
  { value: "mine", label: "我的私房菜" },
  { value: "family", label: "家庭菜谱" },
  { value: "public", label: "公共菜谱" },
]

export default function DishList() {
  const [searchParams] = useSearchParams()
  const requestedScope = searchParams.get("scope") || ""
  const scope = RECIPE_SCOPES.some((item) => item.value === requestedScope) ? requestedScope : ""
  // A navigation from the personal entry to the main tab starts a fresh view.
  return <DishCatalogue key={scope} scope={scope} />
}

function DishCatalogue({ scope }: { scope: string }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const isAdmin = useAuthStore((s) => s.user?.role === "admin")
  const [search, setSearch] = useState("")
  const [activeCategory, setActiveCategory] = useState("全部")
  const [activeTaste, setActiveTaste] = useState("全部")
  const [showTodayModal, setShowTodayModal] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)
  const today = dateKey()

  function changeScope(value: string) {
    setSearch("")
    setActiveCategory("全部")
    setActiveTaste("全部")
    navigate(value ? `/dishes?scope=${value}` : "/dishes")
  }

  const { data: settings } = useQuery({
    queryKey: ["settings"],
    queryFn: () => settingsApi.get(),
  })

  const { data: categoryCountsData } = useQuery({
    queryKey: ["dishes", "category-counts", scope],
    queryFn: () => dishesApi.categoryCounts(scope),
  })
  const categoryCountMap = useMemo(() => {
    const m = new Map<string, number>()
    if (categoryCountsData?.categories) {
      for (const c of categoryCountsData.categories) m.set(c.category, c.count)
    }
    return m
  }, [categoryCountsData])
  const enabledTotal = categoryCountsData?.total ?? 0

  const tastes = useMemo(() => ["全部", ...parseList(settings?.tastes, DEFAULT_TASTES)], [settings])
  const sidebarCats = useMemo(() => {
    const cats = parseList(settings?.categories, DEFAULT_CATEGORIES)
    return ["全部", ...new Set([...cats, ...categoryCountMap.keys()])]
  }, [settings, categoryCountMap])

  const queryParams = useMemo(() => {
    const params: Record<string, string> = {
      pageSize: String(PAGE_SIZE),
      sort: "sort_order",
      order: "asc",
      page: "1",
    }
    if (activeCategory !== "全部") params.category = activeCategory
    if (activeTaste !== "全部") params.taste = activeTaste
    if (search) params.search = search
    if (scope) params.scope = scope
    if (scope !== "mine") params.enabled = "true"
    return params
  }, [activeCategory, activeTaste, search, scope])

  const { data: dishPages, isLoading, error: listError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage, isFetchNextPageError } = useInfiniteQuery({
    queryKey: ["dishes", "list", queryParams],
    initialPageParam: 1,
    queryFn: ({ pageParam }) => dishesApi.list({ ...queryParams, page: String(pageParam) }),
    getNextPageParam: (last, pages) => pages.reduce((sum, page) => sum + page.items.length, 0) < last.total ? pages.length + 1 : undefined,
  })
  const allDishes = useMemo(() => Array.from(new Map((dishPages?.pages.flatMap((page) => page.items) || []).map((dish) => [dish.id, dish])).values()), [dishPages])
  const hasMore = Boolean(hasNextPage)
  const loadMore = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])
  const loadMoreRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const el = loadMoreRef.current
    if (!hasMore || !el || isFetchNextPageError || isFetchingNextPage) return
    const observer = new IntersectionObserver(([entry]) => { if (entry.isIntersecting) loadMore() }, { rootMargin: "200px" })
    observer.observe(el)
    return () => observer.disconnect()
  }, [hasMore, loadMore, isFetchNextPageError, isFetchingNextPage])

  const { data: favDishes } = useQuery({
    queryKey: ["favorites"],
    queryFn: () => favoritesApi.list(),
  })
  const favIds = useMemo(() => new Set((favDishes || []).map((d: Dish) => d.id)), [favDishes])

  const { data: recordsData } = useQuery({
    queryKey: ["records", "date", today],
    queryFn: () => recordsApi.forDates(today, today),
  })
  const todayRecords = useMemo(() => (recordsData?.items || []) as MealRecord[], [recordsData])

  const todayLunchIds = useMemo(() => new Set(
    todayRecords.filter(r => r.meal_type === "lunch" && r.meal_date === today).map(r => r.dish_id)
  ), [todayRecords, today])
  const todayDinnerIds = useMemo(() => new Set(
    todayRecords.filter(r => r.meal_type === "dinner" && r.meal_date === today).map(r => r.dish_id)
  ), [todayRecords, today])

  const favMut = useMutation({
    mutationFn: ({ id, active }: { id: number; active: boolean }) =>
      active ? favoritesApi.remove(id) : favoritesApi.add(id),
    onSuccess: (_, { active }) => {
      qc.invalidateQueries({ queryKey: ["favorites"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success(active ? "已取消收藏" : "❤ 已收藏")
    },
    onError: () => toast.error("操作失败"),
  })

  const recordMut = useMutation({
    mutationFn: (data: { dish_id: number; dish_name: string; meal_type: string; meal_date: string }) =>
      recordsApi.create(data),
    onSuccess: () => {
      toast.success("❤ 已记录！")
      qc.invalidateQueries({ queryKey: ["records"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
    },
    onError: (err: unknown) => {
      const msg = (err instanceof Error ? err.message : "") || "记录失败"
      toast.error(msg)
    },
  })

  const deleteRecordMut = useMutation({
    mutationFn: (id: number) => recordsApi.delete(id),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: ["records"] })
      const prev = qc.getQueriesData({ queryKey: ["records"] })
      qc.setQueriesData({ queryKey: ["records"] }, (old: unknown) => {
        if (!old || typeof old !== "object" || old === null) return old
        const o = old as { items?: MealRecord[] }
        if (!o.items) return old
        return { ...o, items: o.items.filter((r) => r.id !== id) }
      })
      return { prev }
    },
    onSettled: () => { void qc.invalidateQueries({ queryKey: ["records"] }) },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["achievements"] })
    },
    onError: (_err, _id, context) => {
      context?.prev.forEach(([key, value]) => qc.setQueryData(key, value))
      toast.error("删除失败")
    },
  })

  function toggleMeal(dish: Dish, mealType: "lunch" | "dinner") {
    const isAdded = mealType === "lunch" ? todayLunchIds.has(dish.id) : todayDinnerIds.has(dish.id)
    if (isAdded) {
      const record = todayRecords.find(r => r.dish_id === dish.id && r.meal_type === mealType && r.meal_date === today)
      if (record) deleteRecordMut.mutate(record.id)
    } else {
      recordMut.mutate({
        dish_id: dish.id,
        dish_name: dish.name,
        meal_type: mealType,
        meal_date: today,
      })
    }
  }

  const groupedByCategory = useMemo(() => {
    const map = new Map<string, Dish[]>()
    for (const d of allDishes) {
      const cat = d.category || "其他"
      if (!map.has(cat)) map.set(cat, [])
      map.get(cat)!.push(d)
    }
    return map
  }, [allDishes])

  const categoryRefs = useRef<Map<string, HTMLDivElement>>(new Map())
  const [highlightCat, setHighlightCat] = useState("")

  const categoryRefCallback = useCallback((cat: string) => (el: HTMLDivElement | null) => {
    if (el) categoryRefs.current.set(cat, el)
    else categoryRefs.current.delete(cat)
  }, [])

  useEffect(() => {
    const container = listRef.current
    if (!container) return
    const sections = categoryRefs.current
    if (sections.size === 0) return
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setHighlightCat(entry.target.getAttribute("data-category") || "")
          }
        }
      },
      { root: container, rootMargin: "-60px 0px -70% 0px", threshold: 0 }
    )
    sections.forEach((el) => observer.observe(el))
    return () => observer.disconnect()
  }, [allDishes])

  function scrollToCategory(cat: string) {
    setActiveCategory(cat)
    setSearch("")
    if (cat === "全部") {
      gsap.to(listRef.current, { scrollTo: { y: 0 }, duration: 0.36, ease: "power2.out" })
      return
    }
    setTimeout(() => {
      const el = categoryRefs.current.get(cat)
      scrollToElement(el || null, { block: "start", duration: 0.36 })
    }, 100)
  }

  const totalPicked = todayLunchIds.size + todayDinnerIds.size

  const dishMap = useMemo(() => {
    const map = new Map<number, Dish>()
    for (const d of allDishes) map.set(d.id, d)
    return map
  }, [allDishes])

  const todayLunchRecords = useMemo(() =>
    todayRecords.filter(r => r.meal_type === "lunch" && r.meal_date === today),
    [todayRecords, today]
  )
  const todayDinnerRecords = useMemo(() =>
    todayRecords.filter(r => r.meal_type === "dinner" && r.meal_date === today),
    [todayRecords, today]
  )
  const todayMenuCount = todayLunchRecords.length + todayDinnerRecords.length
  const todayLunchCookMinutes = useMemo(() => (
    todayLunchRecords.reduce((sum, record) => {
      const cookTime = dishMap.get(record.dish_id)?.cook_time || 0
      return sum + Math.max(0, cookTime)
    }, 0)
  ), [dishMap, todayLunchRecords])
  const todayDinnerCookMinutes = useMemo(() => (
    todayDinnerRecords.reduce((sum, record) => {
      const cookTime = dishMap.get(record.dish_id)?.cook_time || 0
      return sum + Math.max(0, cookTime)
    }, 0)
  ), [dishMap, todayDinnerRecords])

  const showGrouped = activeCategory === "全部" && activeTaste === "全部" && !search

  return (
    <div className="animate-fadeUp flex flex-col h-full">
      <PageHeader
        title={RECIPE_SCOPES.find((item) => item.value === scope)?.label}
        subtitle={scope === "mine" ? "仅自己可见，共享食谱在家庭查看" : "选菜加餐，一键搞定"}
        icon={UtensilsCrossed}
        actions={
           <HeaderIconButton onClick={() => navigate(isAdmin ? "/admin/dashboard" : "/me/preferences")} aria-label={isAdmin ? "管理设置" : "饮食偏好"}>
            <Settings size={18} strokeWidth={2.3} />
          </HeaderIconButton>
        }
      />

      <div className="relative flex-1 min-h-0 overflow-hidden">
        <div className="absolute inset-0 flex">
          <div className="w-[76px] flex-shrink-0 bg-card border-r border-border overflow-y-hidden scrollbar-none">
            {sidebarCats.map((cat) => {
              const count = cat === "全部" ? enabledTotal : (categoryCountMap.get(cat) ?? 0)
              return (
                <button
                  key={cat}
                  onClick={() => scrollToCategory(cat)}
                  className={`w-full py-3 px-1.5 text-[12px] font-medium text-center transition-all relative whitespace-pre-line leading-tight ${
                    (activeCategory === cat || (cat !== "全部" && highlightCat === cat && activeCategory === "全部"))
                      ? "bg-bg text-primary font-bold before:absolute before:left-0 before:top-1/2 before:-translate-y-1/2 before:w-[3px] before:h-5 before:bg-primary before:rounded-r-full"
                      : "text-text2 bg-card"
                  }`}
                >
                  {cat}<span className="block text-[10px] text-text3 font-normal">{count}道</span>
                </button>
              )
            })}
          </div>

          <div className="flex-1 min-w-0 flex flex-col">
            <div className="px-3 pt-3">
              <label htmlFor="recipe-scope" className="sr-only">菜谱范围</label>
              <select id="recipe-scope" value={scope} onChange={(event) => changeScope(event.target.value)} className="min-h-11 w-full rounded-xl border border-border bg-card px-3 text-sm text-text">
                {RECIPE_SCOPES.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
              </select>
            </div>
            <div className="px-3 pt-3 pb-2">
              <div className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-text3 text-sm">🔍</span>
                <input
                  type="text"
                  placeholder="搜索菜品、配料..."
                  value={search}
                  onChange={(e) => { setSearch(e.target.value); setActiveCategory("全部"); setActiveTaste("全部") }}
                  className="w-full py-2 pl-9 pr-3 rounded-full bg-bg border border-border text-[13px] transition-all focus:border-primary focus:shadow-[0_0_0_3px_rgba(232,115,74,.12)] outline-none"
                />
              </div>
            </div>

            <div className="px-3 pb-2">
              <div className="flex items-center gap-1.5 overflow-x-auto scrollbar-none">
                {tastes.map((t) => (
                  <button
                    key={t}
                    onClick={() => setActiveTaste(t)}
                    className={`px-3 py-1 rounded-full text-[11px] font-medium whitespace-nowrap transition-all active:scale-95 ${
                      activeTaste === t
                        ? "bg-pink text-white"
                        : "bg-card border border-border text-text2"
                    }`}
                  >
                    {t}
                  </button>
                ))}
              </div>
            </div>

            <div ref={listRef} className="app-scroll flex-1 overflow-y-auto px-3 scrollbar-none overscroll-y-contain">
              {isLoading ? (
                <div className="space-y-3">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className="flex gap-3 p-2 rounded-xl">
                      <div className="skeleton w-[72px] h-[72px] rounded-xl flex-shrink-0" />
                      <div className="flex-1 space-y-2 py-1">
                        <div className="skeleton h-3.5 w-3/4 rounded" />
                        <div className="skeleton h-2.5 w-1/2 rounded" />
                      </div>
                    </div>
                  ))}
                </div>
              ) : listError && allDishes.length === 0 ? <RequestState error={listError} onRetry={() => { void refetch() }} compact /> : allDishes.length === 0 ? (
                <div className="text-center py-16">
                  <span className="text-[56px] block mb-4 animate-float">🍽</span>
                  <div className="text-base font-semibold mb-1.5">{scope === "mine" && !search && activeCategory === "全部" && activeTaste === "全部" ? "还没有个人私房菜" : "当前范围没有找到菜品"}</div>
                  <div className="text-[13px] text-text2">{scope === "mine" ? "已共享的做法在家庭菜谱中，公共菜谱可切换到全部查看。" : "可以清除筛选，重新查看全部菜谱。"}</div>
                  <div className="mt-4 flex flex-col items-center gap-2">
                    <button type="button" className="btn-secondary" onClick={() => changeScope("")}>查看全部菜谱</button>
                    {scope === "mine" && <>
                      <button type="button" className="btn-secondary" onClick={() => changeScope("family")}>查看家庭菜谱</button>
                      <button type="button" className="btn-secondary" onClick={() => navigate("/dishes/new")}>创建第一道私房菜</button>
                    </>}
                  </div>
                </div>
              ) : showGrouped ? (
                Array.from(groupedByCategory.entries()).map(([cat, dishes]) => (
                  <div key={cat} ref={categoryRefCallback(cat)} data-category={cat}>
                    <div className="sticky top-0 z-10 glass py-2 px-1 text-[13px] font-bold text-text2 flex items-center gap-2">
                       <span className="w-1 h-4 bg-primary rounded-full" />
                       {cat}
                       <span className="text-[11px] text-text3 font-normal">{categoryCountMap.get(cat) ?? dishes.length}道</span>
                    </div>
                    {dishes.map((d) => (
                      <DishRow
                        key={d.id}
                        dish={d}
                        isFav={favIds.has(d.id)}
                        isLunch={todayLunchIds.has(d.id)}
                        isDinner={todayDinnerIds.has(d.id)}
                        onToggleFav={() => favMut.mutate({ id: d.id, active: favIds.has(d.id) })}
                        onToggleMeal={(mt) => toggleMeal(d, mt)}
                        onClick={() => navigate(`/dishes/${d.id}`)}
                      />
                    ))}
                  </div>
                ))
              ) : (
                allDishes.map((d) => (
                  <DishRow
                    key={d.id}
                    dish={d}
                    isFav={favIds.has(d.id)}
                    isLunch={todayLunchIds.has(d.id)}
                    isDinner={todayDinnerIds.has(d.id)}
                    onToggleFav={() => favMut.mutate({ id: d.id, active: favIds.has(d.id) })}
                    onToggleMeal={(mt) => toggleMeal(d, mt)}
                    onClick={() => navigate(`/dishes/${d.id}`)}
                  />
                ))
              )}

              {hasMore && (
                <div ref={loadMoreRef} className="py-4 text-center text-[12px] text-text3">
                  {isFetchNextPageError ? <button type="button" className="btn-secondary" onClick={loadMore}>加载失败，点击重试</button> : isFetchingNextPage ? "正在加载更多…" : "下滑加载更多"}
                </div>
              )}
              {!hasMore && allDishes.length > 0 && (
                <div className="py-3 text-center text-[11px] text-text3">共 {allDishes.length} 道菜</div>
              )}
            </div>
          </div>
        </div>
      </div>

      {totalPicked > 0 && (
        <div className="flex-shrink-0 glass-strong border-t border-glass-border px-4 py-2.5 flex items-center gap-3 z-20">
          <div className="flex items-center gap-2">
            <div className="relative">
              <ChefHat size={22} className="text-primary" />
              <span className="absolute -top-1.5 -right-2 bg-primary text-white text-[10px] font-bold rounded-full min-w-[16px] h-4 flex items-center justify-center px-1">{totalPicked}</span>
            </div>
            <div className="text-[13px]">
              {todayLunchIds.size > 0 && <span className="text-primary font-semibold">🍳午餐{todayLunchIds.size}道</span>}
              {todayLunchIds.size > 0 && todayDinnerIds.size > 0 && <span className="text-text3 mx-1">·</span>}
              {todayDinnerIds.size > 0 && <span className="text-mint font-semibold">🍲晚餐{todayDinnerIds.size}道</span>}
            </div>
          </div>
          <div className="flex-1" />
          <button
            onClick={() => setShowTodayModal(true)}
            className="py-2 px-5 rounded-full text-[13px] font-semibold bg-primary text-white transition-all active:scale-96"
          >
            今日菜单
          </button>
        </div>
      )}

      {showTodayModal && (
        <AnimatedBottomSheet
          label="今日菜单"
          onClose={() => setShowTodayModal(false)}
          className="flex max-h-[70dvh] flex-col rounded-t-2xl"
        >
          {({ close }) => (
            <>
              <div className="flex items-center justify-between px-5 py-4 border-b border-border">
                <div className="text-base font-bold">📋 今日菜单</div>
                <button onClick={close} aria-label="关闭今日菜单" className="w-11 h-11 rounded-full bg-bg flex items-center justify-center text-text2 active:scale-95 transition-transform">
                  <X size={16} />
                </button>
              </div>

              <div className="app-scroll flex-1 overflow-y-auto px-5 py-3 scrollbar-none">
                <div className="mb-4 grid grid-cols-3 gap-2">
                  <div className="rounded-2xl bg-bg px-3 py-2.5">
                    <div className="text-[11px] font-semibold text-text3">菜品</div>
                    <div className="text-lg font-extrabold leading-tight">{todayMenuCount} 道</div>
                  </div>
                  <div className="rounded-2xl bg-bg px-3 py-2.5">
                    <div className="text-lg font-extrabold leading-tight">午餐 {todayLunchRecords.length} 道</div>
                    <div className="mt-0.5 truncate text-[11px] font-semibold text-text3">
                      约用时 {todayLunchCookMinutes > 0 ? `${todayLunchCookMinutes}m` : "-"}
                    </div>
                  </div>
                  <div className="rounded-2xl bg-bg px-3 py-2.5">
                    <div className="text-lg font-extrabold leading-tight">晚餐 {todayDinnerRecords.length} 道</div>
                    <div className="mt-0.5 truncate text-[11px] font-semibold text-text3">
                      约用时 {todayDinnerCookMinutes > 0 ? `${todayDinnerCookMinutes}m` : "-"}
                    </div>
                  </div>
                </div>

                {todayLunchRecords.length > 0 && (
                  <div className="mb-5">
                    <div className="text-[13px] font-bold text-primary mb-2.5 flex items-center gap-1.5">🍳 午餐</div>
                    <div className="space-y-2">
                      {todayLunchRecords.map((r) => (
                        <TodayMenuItem
                          key={r.id}
                          record={r}
                          dish={dishMap.get(r.dish_id)}
                          onDelete={() => deleteRecordMut.mutate(r.id)}
                          onClick={() => { close(); navigate(`/dishes/${r.dish_id}`) }}
                        />
                      ))}
                    </div>
                  </div>
                )}
                {todayDinnerRecords.length > 0 && (
                  <div className="mb-2">
                    <div className="text-[13px] font-bold text-mint mb-2.5 flex items-center gap-1.5">🍲 晚餐</div>
                    <div className="space-y-2">
                      {todayDinnerRecords.map((r) => (
                        <TodayMenuItem
                          key={r.id}
                          record={r}
                          dish={dishMap.get(r.dish_id)}
                          onDelete={() => deleteRecordMut.mutate(r.id)}
                          onClick={() => { close(); navigate(`/dishes/${r.dish_id}`) }}
                        />
                      ))}
                    </div>
                  </div>
                )}
                {todayLunchRecords.length === 0 && todayDinnerRecords.length === 0 && (
                  <div className="text-center py-8 text-text3 text-sm">还没有选择今日菜品</div>
                )}
              </div>

              <div className="px-5 py-3 border-t border-border" style={{ paddingBottom: "calc(12px + env(safe-area-inset-bottom))" }}>
                <button onClick={close} className="w-full py-2.5 rounded-full text-[13px] font-semibold bg-primary text-white transition-all active:scale-96">
                  继续选菜
                </button>
              </div>
            </>
          )}
        </AnimatedBottomSheet>
      )}
    </div>
  )
}

function TodayMenuItem({ record, dish, onDelete, onClick }: { record: MealRecord; dish?: Dish; onDelete: () => void; onClick: () => void }) {
  const dishHasImage = dish && isImageUrl(dish.image_url)
  const recordHasImage = !dishHasImage && isImageUrl(record.dish_image_url)
  return (
    <div className="flex items-center gap-3 p-2.5 bg-bg rounded-xl">
      <button type="button" onClick={onClick} className="flex min-w-0 flex-1 items-center gap-3 text-left" aria-label={`查看${record.dish_name}`}>
      <div className="w-[52px] h-[52px] rounded-lg overflow-hidden flex-shrink-0 bg-gradient-to-br from-primary-light to-pink-light cursor-pointer">
        {dishHasImage ? (
          <DishImage dish={dish!} className="w-full h-full" emojiSize="text-[20px]" />
        ) : recordHasImage ? (
          <img src={record.dish_image_url} alt={record.dish_name} className="w-full h-full object-cover" />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-[20px]">{record.dish_emoji || "🍽"}</div>
        )}
      </div>
      <div className="flex-1 min-w-0">
        <div className="text-[14px] font-semibold truncate">{record.dish_name}</div>
        <div className="flex items-center gap-1.5 text-[11px] text-text2 mt-0.5">
          {dish?.category && <span>{dish.category}</span>}
          {dish?.cook_time != null && dish.cook_time > 0 && (
            <>
              <span className="w-[3px] h-[3px] rounded-full bg-text3" />
              <span>{dish.cook_time}分钟</span>
            </>
          )}
          {dish?.difficulty && (
            <>
              <span className="w-[3px] h-[3px] rounded-full bg-text3" />
              <span className={diffColor(dish.difficulty)}>{diffLabel(dish.difficulty)}</span>
            </>
          )}
        </div>
      </div>
      </button>
      <button
        onClick={onDelete} aria-label={`从今日菜单移除${record.dish_name}`}
        className="w-11 h-11 rounded-full bg-red-light text-red flex items-center justify-center transition-all active:scale-90 flex-shrink-0"
      >
        <Trash2 size={14} />
      </button>
    </div>
  )
}

interface DishRowProps {
  dish: Dish
  isFav: boolean
  isLunch: boolean
  isDinner: boolean
  onToggleFav: () => void
  onToggleMeal: (mealType: "lunch" | "dinner") => void
  onClick: () => void
}

function DishRow({ dish, isFav, isLunch, isDinner, onToggleFav, onToggleMeal, onClick }: DishRowProps) {
  return (
    <div className="flex items-center gap-3 py-2.5 px-1 rounded-xl transition-all active:bg-card group">
      <button type="button" aria-label={`查看${dish.name}`}
        onClick={onClick}
        className="w-16 h-16 rounded-xl overflow-hidden flex-shrink-0 bg-gradient-to-br from-primary-light to-pink-light cursor-pointer"
      >
        <DishImage dish={dish} className="w-full h-full" emojiSize="text-[28px]" />
      </button>

      <div className="flex-1 min-w-0 relative">
        <div className="flex items-center gap-1 mb-0.5">
          <button type="button" onClick={onClick} className="min-h-11 min-w-0 flex-1 text-left text-[14px] font-semibold"><span className="line-clamp-2">{dish.name}</span></button>
          <button
            onClick={(e) => { e.stopPropagation(); onToggleFav() }}
            aria-label={`${isFav ? "取消收藏" : "收藏"}${dish.name}`} aria-pressed={isFav}
            className={`h-11 w-11 text-[18px] transition-all active:scale-90 flex-shrink-0 ${isFav ? "text-primary" : "text-text3"}`}
          >
            {isFav ? "❤" : "♡"}
          </button>
        </div>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-text2 mb-2">
          <span className="whitespace-nowrap">{dish.category}</span>
          {dish.cook_time > 0 && <span className="whitespace-nowrap">{dish.cook_time}分钟</span>}
          <span className={`whitespace-nowrap ${diffColor(dish.difficulty)}`}>{diffLabel(dish.difficulty)}</span>
          {dish.taste && <span className="max-w-full truncate">{dish.taste}</span>}
        </div>
        <div className="flex items-center gap-2">
          <MealToggle active={isLunch} emoji="🍳" label="午餐" onClick={() => onToggleMeal("lunch")} color="primary" />
          <MealToggle active={isDinner} emoji="🍲" label="晚餐" onClick={() => onToggleMeal("dinner")} color="mint" />
        </div>
      </div>
    </div>
  )
}

interface MealToggleProps {
  active: boolean
  emoji: string
  label: string
  onClick: () => void
  color: "primary" | "mint"
}

function MealToggle({ active, emoji, label, onClick, color }: MealToggleProps) {
  const activeClass = color === "primary"
    ? "bg-primary text-white border-primary"
    : "bg-mint text-white border-mint"
  const inactiveClass = "bg-card border-border text-text2"

  return (
    <button
      onClick={(e) => { e.stopPropagation(); onClick() }}
      aria-pressed={active} aria-label={`${active ? "移出" : "加入"}${label}`}
      className={`flex flex-1 min-h-11 min-w-0 justify-center items-center gap-1 px-1.5 py-1 whitespace-nowrap rounded-full text-[11px] font-semibold border transition-all active:scale-95 ${
        active ? activeClass : inactiveClass
      }`}
    >
      {active ? <Minus size={10} /> : <Plus size={10} />}
      <span>{emoji}</span>
      <span>{label}</span>
    </button>
  )
}
