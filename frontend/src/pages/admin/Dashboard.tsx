import RequestState from "@/components/RequestState"
import { useNavigate } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { dashboardApi } from "@/api"

function moodLabel(m: string) {
  if (m === "yum" || m === "great") return "😋"
  if (m === "ok") return "😐"
  if (m === "no" || m === "meh") return "😒"
  return ""
}

function mealLabel(t: string) {
  return t === "lunch" ? "午餐" : "晚餐"
}

function diffLabel(d: string) { return d === "easy" ? "简单" : d === "medium" ? "中等" : "困难" }

export default function AdminDashboard() {
  const navigate = useNavigate()
  const { data: d, isLoading , error: loadError, refetch: retryLoad } = useQuery({
    queryKey: ["admin", "dashboard"],
    queryFn: () => dashboardApi.get(),
  })

  if (loadError) return <div><RequestState error={loadError} onRetry={() => { void retryLoad() }} /></div>

  if (isLoading) return <div className="p-8 text-center text-text2">加载中...</div>

  const trend = d?.week_trend || []
  const maxTrend = Math.max(...trend.map((w) => w.count), 1)

  return (
    <div className="px-5 py-4 max-w-[640px] mx-auto pb-20">
      <div className="grid grid-cols-3 gap-2.5 mb-5">
        <div className="bg-card rounded-2xl p-3 shadow-sm text-center">
          <div className="text-xl font-extrabold">{d?.total_dishes || 0}</div>
          <div className="text-[11px] text-text2">菜品总数</div>
        </div>
        <div className="bg-card rounded-2xl p-3 shadow-sm text-center">
          <div className="text-xl font-extrabold text-mint">{d?.enabled_dishes || 0}</div>
          <div className="text-[11px] text-text2">启用中</div>
        </div>
        <div className="bg-card rounded-2xl p-3 shadow-sm text-center">
          <div className="text-xl font-extrabold text-text3">{d?.disabled_dishes || 0}</div>
          <div className="text-[11px] text-text2">已禁用</div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2.5 mb-5">
        <div className="bg-card rounded-2xl p-3 shadow-sm text-center">
          <div className="text-xl font-extrabold">{d?.today_records || 0}</div>
          <div className="text-[11px] text-text2">今日点菜</div>
        </div>
        <div className="bg-card rounded-2xl p-3 shadow-sm text-center">
          <div className="text-xl font-extrabold">{d?.total_records || 0}</div>
          <div className="text-[11px] text-text2">总记录</div>
        </div>
      </div>

      <div className="mb-5">
        <div className="text-sm font-bold mb-2">近7天点菜趋势</div>
        <div className="bg-card rounded-2xl p-4 shadow-sm">
          <div className="flex items-end gap-1.5 h-20">
            {trend.map((w, i) => (
              <div key={i} className="flex-1 flex flex-col items-center gap-1">
                <span className="text-[10px] text-text2 font-semibold">{w.count}</span>
                <div
                  className="w-full rounded-t-md bg-primary/70 transition-all"
                  style={{ height: `${Math.max((w.count / maxTrend) * 48, 3)}px` }}
                />
                <span className="text-[9px] text-text3">{w.date.slice(5)}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="mb-5">
        <div className="text-sm font-bold mb-2">分类分布</div>
        <div className="bg-card rounded-2xl p-4 shadow-sm">
          <div className="flex flex-wrap gap-2">
            {(d?.category_counts || []).map((c) => (
              <span key={c.category} className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs bg-primary-light text-primary font-semibold">
                {c.category}<span className="text-text2">{c.count}</span>
              </span>
            ))}
          </div>
          {(d?.difficulty_counts || []).length > 0 && (
            <div className="flex flex-wrap gap-2 mt-3 pt-3 border-t border-border">
              {(d?.difficulty_counts || []).map((c) => (
                <span key={c.difficulty} className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs bg-bg text-text2 font-semibold">
                  {diffLabel(c.difficulty)}<span className="text-text3">{c.count}</span>
                </span>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="mb-5">
        <div className="text-sm font-bold mb-2">热门菜品 TOP8</div>
        <div className="bg-card rounded-2xl overflow-hidden shadow-sm border border-border">
          {(d?.top_dishes || []).length === 0 && (
            <div className="py-8 text-center text-sm text-text3">暂无记录数据</div>
          )}
          {(d?.top_dishes || []).map((t, i) => (
            <div key={t.dish_id} onClick={() => navigate(`/admin/dishes/${t.dish_id}`)} className="flex items-center gap-3 px-4 py-2.5 border-b border-border last:border-0 cursor-pointer transition-all active:bg-bg">
              <div className={`w-6 h-6 rounded-full flex items-center justify-center text-xs font-bold flex-shrink-0 ${i < 3 ? "bg-primary text-white" : "bg-bg text-text3"}`}>
                {i + 1}
              </div>
              <div className="flex-1 text-sm font-medium truncate">{t.dish_name}</div>
              <div className="text-xs text-primary font-semibold">{t.count}次</div>
            </div>
          ))}
        </div>
      </div>

      <div className="mb-5">
        <div className="text-sm font-bold mb-2">最近记录</div>
        <div className="bg-card rounded-2xl overflow-hidden shadow-sm border border-border">
          {(d?.recent_records || []).length === 0 && (
            <div className="py-8 text-center text-sm text-text3">暂无记录</div>
          )}
          {(d?.recent_records || []).map((r) => (
            <div key={r.id} className="flex items-center gap-3 px-4 py-2.5 border-b border-border last:border-0">
              <span className="text-base">{moodLabel(r.mood)}</span>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate">{r.dish_name}</div>
                <div className="text-[11px] text-text2">{r.meal_date} · {mealLabel(r.meal_type)}</div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div>
        <div className="text-sm font-bold mb-2">快捷操作</div>
        <div className="grid grid-cols-2 gap-2.5">
          <button onClick={() => navigate("/admin/dishes/new")} className="bg-card rounded-2xl p-4 shadow-sm border border-border text-left transition-all active:scale-99">
            <div className="text-2xl mb-1.5">➕</div>
            <div className="text-sm font-semibold">添加菜品</div>
            <div className="text-[11px] text-text2">新增一道菜谱</div>
          </button>
          <button onClick={() => navigate("/admin/dishes")} className="bg-card rounded-2xl p-4 shadow-sm border border-border text-left transition-all active:scale-99">
            <div className="text-2xl mb-1.5">📋</div>
            <div className="text-sm font-semibold">管理菜品</div>
            <div className="text-[11px] text-text2">搜索筛选批量操作</div>
          </button>
          <button onClick={() => navigate("/admin/records")} className="bg-card rounded-2xl p-4 shadow-sm border border-border text-left transition-all active:scale-99">
            <div className="text-2xl mb-1.5">📋</div>
            <div className="text-sm font-semibold">用餐记录</div>
            <div className="text-[11px] text-text2">查看和管理记录</div>
          </button>
          <button onClick={() => navigate("/admin/quotes")} className="bg-card rounded-2xl p-4 shadow-sm border border-border text-left transition-all active:scale-99">
            <div className="text-2xl mb-1.5">💬</div>
            <div className="text-sm font-semibold">语录管理</div>
            <div className="text-[11px] text-text2">推荐语录增删改</div>
          </button>
        </div>
      </div>
    </div>
  )
}
