import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Check, Clock3, Inbox, X } from "lucide-react"
import { useNavigate } from "react-router-dom"
import toast from "react-hot-toast"
import { errorMessage, suggestionsApi } from "@/api"
import DishImage from "@/components/DishImage"
import PageHeader from "@/components/PageHeader"

const dateLabel = (raw: string) => raw ? new Date(`${raw.slice(0, 10)}T12:00:00`).toLocaleDateString("zh-CN", { month: "short", day: "numeric" }) : "未指定日期"

export default function Suggestions() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [showHistory, setShowHistory] = useState(false)
  const status = showHistory ? "all" : "pending"
  const { data: suggestions = [], isLoading, isError } = useQuery({ queryKey: ["suggestions", status], queryFn: () => suggestionsApi.list(status) })
  const resolveMutation = useMutation({
    mutationFn: ({ id, accept, meal_type }: { id: number; accept: boolean; meal_type?: "lunch" | "dinner" }) => {
      const suggestion = suggestions.find((item) => item.id === id)
      return suggestionsApi.resolve(id, { accept, meal_type, dish_ids: accept ? suggestion?.dishes.map((dish) => dish.id) : undefined })
    },
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({ queryKey: ["suggestions"] })
      void queryClient.invalidateQueries({ queryKey: ["records"] })
      void queryClient.invalidateQueries({ queryKey: ["stats"] })
      toast.success(variables.accept ? "已加入用餐记录" : "已忽略这条建议")
    },
    onError: (error) => toast.error(errorMessage(error, "操作失败")),
  })

  return (
    <div className="min-h-dvh bg-bg pb-8">
      <PageHeader title="AI 建议" subtitle="由你决定是否采纳" icon={Inbox} onBack={() => navigate(-1)} actions={<button onClick={() => setShowHistory((value) => !value)} className="rounded-md px-2 py-1 text-xs font-semibold text-text2 hover:bg-card">{showHistory ? "待处理" : "历史"}</button>} />
      <main className="mx-auto max-w-[640px] space-y-3 px-5 py-5">
        {isLoading ? <div className="h-36 animate-pulse rounded-lg bg-card" /> : isError ? (
          <div role="alert" className="rounded-lg border border-border bg-card p-5 text-sm text-text2">建议暂时无法加载，请稍后重试。</div>
        ) : suggestions.length === 0 ? (
          <div className="rounded-lg border border-border bg-card px-5 py-12 text-center">
            <Inbox size={27} className="mx-auto text-text3" />
            <p className="mt-3 text-sm font-semibold">{showHistory ? "还没有建议记录" : "暂时没有待处理建议"}</p>
            <p className="mt-1 text-xs text-text3">第三方智能体推送的菜单会出现在这里</p>
          </div>
        ) : suggestions.map((suggestion) => (
          <article key={suggestion.id} className="overflow-hidden rounded-lg border border-border bg-card">
            <div className="flex items-start justify-between gap-3 px-4 py-3.5">
              <div className="min-w-0">
                <h2 className="truncate text-sm font-bold">{suggestion.title}</h2>
                <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-text3">
                  <span>{suggestion.source || "AI 助手"}</span>
                  <span className="inline-flex items-center gap-1"><Clock3 size={11} />{dateLabel(suggestion.meal_date)}</span>
                  <span className="rounded bg-bg px-1.5 py-0.5">{suggestion.status === "pending" ? "待处理" : suggestion.status === "accepted" ? "已采纳" : suggestion.status === "dismissed" ? "已忽略" : "已过期"}</span>
                </div>
              </div>
              {suggestion.status === "pending" && <button onClick={() => resolveMutation.mutate({ id: suggestion.id, accept: false })} aria-label="忽略建议" title="忽略建议" disabled={resolveMutation.isPending} className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-text3 hover:bg-bg hover:text-red"><X size={16} /></button>}
            </div>
            {suggestion.reason && <p className="px-4 pb-3 text-[13px] leading-6 text-text2">{suggestion.reason}</p>}
            {suggestion.dishes.length > 0 && <div className="divide-y divide-border border-y border-border">
              {suggestion.dishes.map((dish) => <button key={dish.id} onClick={() => navigate(`/dishes/${dish.id}`)} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-bg/70"><span className="h-11 w-11 shrink-0 overflow-hidden rounded-md bg-bg"><DishImage dish={dish} className="h-full w-full" /></span><span className="min-w-0 flex-1"><span className="block truncate text-sm font-semibold">{dish.name}</span><span className="text-[11px] text-text3">{dish.category} · {dish.cook_time || "-"} 分钟</span></span></button>)}
            </div>}
            {suggestion.status === "pending" && <div className="grid grid-cols-2 gap-2 p-3">
              <button onClick={() => resolveMutation.mutate({ id: suggestion.id, accept: true, meal_type: "lunch" })} disabled={resolveMutation.isPending || suggestion.dishes.length === 0} className="flex h-10 items-center justify-center gap-1.5 rounded-md bg-primary text-xs font-semibold text-white disabled:opacity-50"><Check size={14} />记为午餐</button>
              <button onClick={() => resolveMutation.mutate({ id: suggestion.id, accept: true, meal_type: "dinner" })} disabled={resolveMutation.isPending || suggestion.dishes.length === 0} className="flex h-10 items-center justify-center gap-1.5 rounded-md bg-mint text-xs font-semibold text-white disabled:opacity-50"><Check size={14} />记为晚餐</button>
            </div>}
          </article>
        ))}
      </main>
    </div>
  )
}
