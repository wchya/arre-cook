import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Activity, ArrowRight, Bot, Plus, Trash2 } from "lucide-react"
import toast from "react-hot-toast"
import { errorMessage, healthApi } from "@/api"
import PageHeader from "@/components/PageHeader"
import { getDishImageUrl } from "@/lib/dish-image"
import type { FoodJournalInput } from "@/types"

const meals = [
  { value: "breakfast", label: "早餐" }, { value: "lunch", label: "午餐" },
  { value: "dinner", label: "晚餐" }, { value: "snack", label: "加餐" },
]
const groups = [
  { value: "vegetable", label: "蔬菜" }, { value: "fruit", label: "水果" },
  { value: "protein", label: "蛋白质食物" }, { value: "whole_grain", label: "全谷物" },
  { value: "dairy", label: "奶类" },
]
const cuisines = ["家常", "川菜", "湘菜", "粤菜", "鲁菜", "苏菜", "浙菜", "闽菜", "徽菜", "东北菜", "西餐", "日料", "韩餐"]

function today() {
  const date = new Date()
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
}
function empty(): FoodJournalInput {
  return { meal_date: today(), meal_type: "lunch", dish_name: "", cuisine: "", food_groups: [], notes: "" }
}

export default function Health() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [view, setView] = useState<"journal" | "report">("journal")
  const [days, setDays] = useState<7 | 30>(7)
  const [form, setForm] = useState<FoodJournalInput>(empty)
  const [showForm, setShowForm] = useState(false)
  const [busy, setBusy] = useState(false)
  const { data: journal = [], isLoading: journalLoading } = useQuery({ queryKey: ["food-journal"], queryFn: () => healthApi.journal(), enabled: view === "journal" })
  const { data: report, isLoading: reportLoading } = useQuery({ queryKey: ["health-report", days], queryFn: () => healthApi.report(days), enabled: view === "report" })

  async function save() {
    if (!form.dish_name.trim() || busy) return
    setBusy(true)
    try {
      await healthApi.record({ ...form, dish_name: form.dish_name.trim(), cuisine: form.cuisine.trim(), notes: form.notes.trim() })
      await Promise.all([qc.invalidateQueries({ queryKey: ["food-journal"] }), qc.invalidateQueries({ queryKey: ["health-report"] })])
      setForm(empty())
      setShowForm(false)
      toast.success("已记下这餐")
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function remove(id: number) {
    if (!confirm("删除这条饮食记录？")) return
    try {
      await healthApi.remove(id)
      await Promise.all([qc.invalidateQueries({ queryKey: ["food-journal"] }), qc.invalidateQueries({ queryKey: ["health-report"] })])
      toast.success("已删除")
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return <div className="min-h-dvh bg-bg pb-16 text-text">
    <PageHeader title="饮食记录" subtitle="实际吃过的，才进入报告" icon={Activity} onBack={() => navigate(-1)} />
    <main className="mx-auto max-w-[640px] px-5 py-5">
      <div className="mb-6 grid grid-cols-2 border-b border-border" role="tablist" aria-label="饮食记录视图">
        <button role="tab" aria-selected={view === "journal"} onClick={() => setView("journal")} className={`h-11 border-b-2 text-sm font-semibold ${view === "journal" ? "border-primary text-primary" : "border-transparent text-text3"}`}>饮食日记</button>
        <button role="tab" aria-selected={view === "report"} onClick={() => setView("report")} className={`h-11 border-b-2 text-sm font-semibold ${view === "report" ? "border-primary text-primary" : "border-transparent text-text3"}`}>饮食报告</button>
      </div>

      {view === "journal" ? <div>
        <button onClick={() => setShowForm((value) => !value)} className="mb-5 flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-primary text-sm font-semibold text-white"><Plus size={18} />记录一餐</button>
        {showForm && <form onSubmit={(e) => { e.preventDefault(); void save() }} className="mb-6 space-y-4 border-y border-border bg-card py-5">
          <div className="grid grid-cols-2 gap-3"><label className="text-xs font-semibold text-text2">日期<input type="date" required value={form.meal_date} max={today()} onChange={(e) => setForm({ ...form, meal_date: e.target.value })} className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text" /></label><label className="text-xs font-semibold text-text2">餐次<select value={form.meal_type} onChange={(e) => setForm({ ...form, meal_type: e.target.value })} className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text">{meals.map((meal) => <option key={meal.value} value={meal.value}>{meal.label}</option>)}</select></label></div>
          <label className="block text-xs font-semibold text-text2">吃了什么<input required maxLength={100} value={form.dish_name} onChange={(e) => setForm({ ...form, dish_name: e.target.value })} placeholder="例如：番茄鸡蛋面" className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text outline-none focus:border-primary" /></label>
          <label className="block text-xs font-semibold text-text2">菜系（可选）<input maxLength={40} list="cuisine-options" value={form.cuisine} onChange={(e) => setForm({ ...form, cuisine: e.target.value })} placeholder="自己填写或选择" className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text outline-none focus:border-primary" /><datalist id="cuisine-options">{cuisines.map((name) => <option key={name} value={name} />)}</datalist></label>
          <fieldset><legend className="mb-2 text-xs font-semibold text-text2">包含哪些食物类别（可选）</legend><div className="flex flex-wrap gap-2">{groups.map((group) => { const selected = form.food_groups.includes(group.value); return <label key={group.value} className={`cursor-pointer rounded-lg border px-3 py-2 text-xs font-medium ${selected ? "border-mint bg-mint-light text-mint" : "border-border bg-bg text-text2"}`}><input type="checkbox" checked={selected} onChange={() => setForm({ ...form, food_groups: selected ? form.food_groups.filter((value) => value !== group.value) : [...form.food_groups, group.value] })} className="sr-only" />{group.label}</label> })}</div></fieldset>
          <label className="block text-xs font-semibold text-text2">备注（可选）<textarea rows={2} maxLength={500} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} className="mt-1 w-full resize-none rounded-lg border border-border bg-bg p-3 text-sm text-text" /></label>
          <div className="flex gap-2"><button type="button" onClick={() => setShowForm(false)} className="h-11 flex-1 rounded-lg border border-border text-sm font-semibold">取消</button><button disabled={!form.dish_name.trim() || busy} className="h-11 flex-1 rounded-lg bg-primary text-sm font-semibold text-white disabled:opacity-50">保存记录</button></div>
        </form>}
        <div className="mb-4 flex items-center justify-between"><h2 className="text-base font-bold">手动记录</h2><button onClick={() => navigate("/history")} className="flex items-center gap-1 text-xs font-semibold text-primary">查看菜谱用餐记录 <ArrowRight size={14} /></button></div>
        {journalLoading ? <div className="skeleton h-24 rounded-lg" /> : journal.length === 0 ? <p className="py-8 text-center text-sm text-text3">还没有手动记录，今天吃了什么？</p> : <div className="divide-y divide-border border-y border-border">{journal.map((entry) => <div key={entry.id} className="flex gap-3 py-3"><div className="min-w-0 flex-1"><div className="text-xs text-text3">{entry.meal_date} · {meals.find((meal) => meal.value === entry.meal_type)?.label}</div><div className="mt-0.5 text-sm font-semibold">{entry.dish_name}</div><div className="mt-1 text-xs text-text2">{[entry.cuisine, ...entry.food_groups.map((value) => groups.find((group) => group.value === value)?.label || value)].filter(Boolean).join(" · ")}</div>{entry.notes && <p className="mt-1 text-xs text-text3">{entry.notes}</p>}</div><button onClick={() => void remove(entry.id)} title="删除记录" aria-label={`删除${entry.dish_name}`} className="self-start p-2 text-text3"><Trash2 size={16} /></button></div>)}</div>}
      </div> : <div>
        <div className="mb-5 inline-flex rounded-lg border border-border bg-card p-1" role="group" aria-label="报告时间范围">{([7, 30] as const).map((value) => <button key={value} onClick={() => setDays(value)} className={`h-9 min-w-20 rounded-md px-3 text-xs font-semibold ${days === value ? "bg-primary text-white" : "text-text3"}`}>近 {value} 天</button>)}</div>
        {reportLoading ? <div className="skeleton h-40 rounded-lg" /> : report && <div className="space-y-7">
          <section><h2 className="mb-3 text-base font-bold">记录概览</h2><div className="grid grid-cols-2 gap-3 border-y border-border py-4"><div><div className="text-2xl font-bold text-primary">{report.logged_days}<span className="ml-1 text-xs font-medium text-text3">/ {days} 天</span></div><div className="text-xs text-text3">有记录的日子</div></div><div><div className="text-2xl font-bold text-mint">{report.meal_count}</div><div className="text-xs text-text3">饮食记录条目</div></div></div><p className="mt-2 text-xs leading-relaxed text-text3">统计包含菜谱用餐记录和手动记录；两边都记了同一餐会分别计入。未记录不代表未吃。</p></section>
          <section><h2 className="mb-3 text-base font-bold">这段时间的变化</h2><div className="space-y-2">{report.insights.map((text, index) => <p key={index} className="border-l-2 border-mint pl-3 text-sm leading-relaxed text-text2">{text}</p>)}</div>{report.cuisine_counts.length > 0 && <div className="mt-4 flex flex-wrap gap-2">{report.cuisine_counts.slice(0, 8).map((item) => <span key={item.name} className="rounded-md bg-card px-2.5 py-1.5 text-xs text-text2">{item.name} · {item.count}</span>)}</div>}</section>
          {Object.keys(report.food_group_days).length > 0 && <section><h2 className="mb-3 text-base font-bold">已标注的食物类别</h2><div className="grid grid-cols-2 gap-x-5 gap-y-3 border-y border-border py-3">{groups.map((group) => <div key={group.value} className="flex justify-between gap-2 text-sm"><span className="text-text2">{group.label}</span><span className="font-semibold">{report.food_group_days[group.value] || 0} 天</span></div>)}</div><p className="mt-2 text-xs text-text3">仅统计手动标注的日子；未标注不代表没有吃。</p></section>}
          <section><h2 className="mb-3 text-base font-bold">接下来这样安排</h2><ol className="space-y-3">{report.plan_actions.map((action, index) => <li key={index} className="flex gap-3 text-sm leading-relaxed text-text2"><span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary-light text-xs font-bold text-primary">{index + 1}</span>{action}</li>)}</ol><button onClick={() => navigate("/plan")} className="mt-4 flex items-center gap-1 text-sm font-semibold text-primary">安排个人一周菜单 <ArrowRight size={15} /></button></section>
          {report.recommendations.length > 0 && <section><h2 className="mb-3 text-base font-bold">换个口味</h2><div className="divide-y divide-border border-y border-border">{report.recommendations.map(({ dish, reason }) => <button key={dish.id} onClick={() => navigate(`/dishes/${dish.id}`)} className="flex w-full items-center gap-3 py-3 text-left">{getDishImageUrl(dish) ? <img src={getDishImageUrl(dish)!} alt="" className="h-14 w-14 shrink-0 rounded-md object-cover" /> : <div className="h-14 w-14 shrink-0 rounded-md bg-primary-light" />}<div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{dish.name}</div><div className="mt-1 text-xs leading-relaxed text-text3">{reason}</div></div><ArrowRight size={16} className="shrink-0 text-text3" /></button>)}</div></section>}
          <button onClick={() => navigate("/assistant")} className="flex h-11 w-full items-center justify-center gap-2 rounded-lg border border-border bg-card text-sm font-semibold text-text2"><Bot size={17} />和 AI 助手聊饮食规划</button>
          <p className="text-xs leading-relaxed text-text3">报告用于整理你填写的饮食信息，不提供营养诊断或医疗建议。</p>
        </div>}
      </div>}
    </main>
  </div>
}
