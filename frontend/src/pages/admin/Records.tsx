import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { recordsApi } from "@/api"
import type { MealRecord } from "@/types"
import toast from "react-hot-toast"

function moodEmoji(m: string) {
  if (m === "yum" || m === "great") return "😋"
  if (m === "ok") return "😐"
  if (m === "no" || m === "meh") return "😒"
  return "🍽"
}

function mealLabel(t: string) {
  return t === "lunch" ? "午餐" : "晚餐"
}

export default function AdminRecords() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [filterMealType, setFilterMealType] = useState("")
  const [dateFrom, setDateFrom] = useState("")
  const [dateTo, setDateTo] = useState("")
  const pageSize = 30

  const params: Record<string, string> = { pageSize: String(pageSize), page: String(page) }
  if (filterMealType) params.meal_type = filterMealType
  if (dateFrom) params.date_from = dateFrom
  if (dateTo) params.date_to = dateTo

  const { data: recordsData, isLoading } = useQuery({
    queryKey: ["admin", "records", params],
    queryFn: () => recordsApi.list(params),
  })
  const records = recordsData?.items || []
  const total = recordsData?.total || 0
  const totalPages = Math.ceil(total / pageSize)

  const deleteMut = useMutation({
    mutationFn: (id: number) => recordsApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "records"] })
      toast.success("已删除")
    },
  })

  const pillClass = (active: boolean) =>
    `px-3 py-1.5 rounded-full text-xs border-[1.5px] transition-all ${active ? "bg-primary text-white border-primary" : "border-border text-text2"}`

  return (
    <div className="px-5 py-4 max-w-[640px] mx-auto pb-20">
      <div className="flex items-center justify-between mb-3">
        <div className="text-lg font-bold">用餐记录</div>
        <span className="text-xs text-text3">共 {total} 条</span>
      </div>

      <div className="flex flex-wrap gap-1.5 mb-3">
        <button onClick={() => { setFilterMealType(""); setPage(1) }} className={pillClass(filterMealType === "")}>全部</button>
        <button onClick={() => { setFilterMealType("lunch"); setPage(1) }} className={pillClass(filterMealType === "lunch")}>午餐</button>
        <button onClick={() => { setFilterMealType("dinner"); setPage(1) }} className={pillClass(filterMealType === "dinner")}>晚餐</button>
      </div>

      <div className="bg-card rounded-2xl p-3 shadow-sm border border-border mb-3">
        <div className="flex items-center gap-2">
          <input type="date" value={dateFrom} onChange={(e) => { setDateFrom(e.target.value); setPage(1) }} className="flex-1 min-w-0 py-2 px-2.5 rounded-lg border-[1.5px] border-border bg-bg text-xs outline-none focus:border-primary" />
          <span className="text-xs text-text3">—</span>
          <input type="date" value={dateTo} onChange={(e) => { setDateTo(e.target.value); setPage(1) }} className="flex-1 min-w-0 py-2 px-2.5 rounded-lg border-[1.5px] border-border bg-bg text-xs outline-none focus:border-primary" />
          {(dateFrom || dateTo) && (
            <button onClick={() => { setDateFrom(""); setDateTo(""); setPage(1) }} className="text-[11px] text-primary font-semibold flex-shrink-0">清除</button>
          )}
        </div>
      </div>

      {isLoading ? (
        <div className="py-12 text-center text-text2">加载中...</div>
      ) : (
        <div className="bg-card rounded-2xl overflow-hidden shadow-sm border border-border">
          {records.map((r: MealRecord) => (
            <div key={r.id} className="flex items-center gap-2.5 px-4 py-2.5 border-b border-border last:border-0">
              <span className="text-base flex-shrink-0">{moodEmoji(r.mood)}</span>
              <div className="flex-1 min-w-0 overflow-hidden">
                <div className="text-sm font-semibold truncate">{r.dish_name}</div>
                <div className="text-[11px] text-text2 truncate">
                  {r.meal_date} · {mealLabel(r.meal_type)}
                  {r.rating > 0 && <span className="ml-1">评分{r.rating}</span>}
                  {r.remark && <span className="ml-1">· {r.remark}</span>}
                </div>
              </div>
              {r.photo && (
                <img src={r.photo} alt="" className="w-9 h-9 rounded-lg object-cover flex-shrink-0" />
              )}
              <button
                onClick={() => { if (confirm("确定删除此记录？")) deleteMut.mutate(r.id) }}
                className="w-7 h-7 rounded-lg flex items-center justify-center text-xs bg-bg transition-all active:bg-red-light active:text-red-500 flex-shrink-0"
              >
                🗑
              </button>
            </div>
          ))}
          {records.length === 0 && (
            <div className="py-12 text-center text-sm text-text3">暂无记录</div>
          )}
        </div>
      )}

      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-2 mt-4">
          <button onClick={() => setPage(1)} disabled={page <= 1} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">首页</button>
          <button onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page <= 1} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">上一页</button>
          <span className="text-xs text-text2">{page}/{totalPages}</span>
          <button onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page >= totalPages} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">下一页</button>
          <button onClick={() => setPage(totalPages)} disabled={page >= totalPages} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">末页</button>
        </div>
      )}
    </div>
  )
}
