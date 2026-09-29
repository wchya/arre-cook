import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router-dom"
import { healthApi } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import { healthDate } from "@/lib/health-date"

export default function HealthSummaryCard() {
  const userId = useAuthStore(state => state.user?.id)
  const { data, error, isPending, refetch } = useQuery({
    queryKey: ["health-report", "summary", userId, healthDate()],
    queryFn: () => healthApi.summary(), enabled: !!userId, staleTime: 0,
  })
  return <section className="mb-6 rounded-2xl border border-border bg-card p-4" aria-label="最近七天饮食摘要">
    <div className="flex items-center justify-between gap-3"><h2 className="text-base font-bold">这一周，吃得有记录</h2><Link to="/health?view=report" className="inline-flex min-h-11 shrink-0 items-center text-sm font-semibold text-primary">查看周报 →</Link></div>
    {error ? <div role="status" className="text-sm text-text2">周报摘要暂时无法读取。<button className="min-h-11 px-2 underline" onClick={() => void refetch()}>重试</button></div>
      : isPending ? <p role="status" className="min-h-12 text-sm text-text2">正在整理最近七天的记录…</p>
      : data && <>
        <p className="text-xs leading-5 text-text2">{data.from} — {data.to} · 北京时间</p>
        <p className="mt-2 text-sm leading-6">{data.logged_days} 天有实际饮食记录 · {data.complete_days} 天确认完整</p>
        <p className="text-sm leading-6 text-text2">{data.meal_event_count} 餐 · {data.item_count} 个食物项{data.not_eaten_meals > 0 ? ` · ${data.not_eaten_meals} 餐明确未吃` : ""}</p>
        <p className="mt-2 text-sm leading-6 text-text2">{data.headline}。未记录不代表没有吃。</p>
      </>}
  </section>
}
