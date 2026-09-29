import RequestState from "@/components/RequestState"
import { useAuthStore } from "@/store/useAuthStore"
import { useState } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Activity, ArrowRight, Plus, Trash2 } from "lucide-react"
import toast from "react-hot-toast"
import { errorMessage, healthApi } from "@/api"
import PageHeader from "@/components/PageHeader"
import NutritionEntryFields from "@/components/NutritionEntryFields"
import HealthProfilePanel from "@/components/HealthProfilePanel"
import HealthMealDraftPanel from "@/components/HealthMealDraftPanel"
import HealthReportPanel from "@/components/HealthReportPanel"
import { healthDate } from "@/lib/health-date"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import type { FoodJournalEntry, FoodJournalInput } from "@/types"

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

function today() { return healthDate() }
function empty(): FoodJournalInput {
  return { meal_date: today(), meal_type: "lunch", dish_name: "", cuisine: "", food_groups: [], notes: "", portion: "", request_key: crypto.randomUUID() }
}

export default function Health() {
  const userId = useAuthStore(state => state.user?.id)
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [params] = useSearchParams()
  const [view, setView] = useState<"journal" | "report">(params.get("view") === "report" ? "report" : "journal")
  const [days, setDays] = useState<7 | 30>(7)
  const [form, setForm] = useState<FoodJournalInput>(empty)
  const [showForm, setShowForm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [nutritionBusy, setNutritionBusy] = useState(false)
  const [editId, setEditId] = useState<number | null>(null)
  function edit(entry: FoodJournalEntry, copy = false) { setEditId(copy ? null : entry.id); setForm({ ...entry, meal_date: copy ? today() : entry.meal_date, nutrition: copy ? null : entry.nutrition, nutrition_mode: copy ? "clear" : "keep", linked_record_id: copy ? null : entry.linked_record_id, event_key: copy ? "" : entry.event_key, request_key: crypto.randomUUID() }); setShowForm(true) }
  const { data: journal = [], isLoading: journalLoading, error: journalError, refetch: reloadJournal } = useQuery({ queryKey: ["food-journal", userId], queryFn: () => healthApi.journal(), enabled: view === "journal" })
  const { data: report, isLoading: reportLoading, error: reportError, refetch: reloadReport } = useQuery({ queryKey: ["health-report", days, userId], queryFn: () => healthApi.report(days), enabled: view === "report" })

  async function save() {
    if (!form.dish_name.trim() || busy || nutritionBusy) return
    setBusy(true)
    const epoch = getSessionEpoch()
    try {
      const input = { ...form, dish_name: form.dish_name.trim(), cuisine: form.cuisine.trim(), notes: form.notes.trim() }
      if (editId) await healthApi.update(editId, input); else await healthApi.record(input)
      if (!isCurrentSession(epoch)) return
      await Promise.all([qc.invalidateQueries({ queryKey: ["food-journal"] }), qc.invalidateQueries({ queryKey: ["health-report"] })])
      if (!isCurrentSession(epoch)) return
      setForm(empty())
      setEditId(null)
      setShowForm(false)
      toast.success("已记下这餐")
    } catch (err) {
      if (isCurrentSession(epoch)) toast.error(errorMessage(err))
    } finally {
      if (isCurrentSession(epoch)) setBusy(false)
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

      <HealthProfilePanel key={userId} />

      {view === "journal" ? <div>
        <HealthMealDraftPanel key={userId} />
        <button onClick={() => { setEditId(null); setForm(empty()); setShowForm(true) }} className="mb-5 flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-primary text-sm font-semibold text-white"><Plus size={18} />记录一餐</button>
        {showForm && <form onSubmit={(e) => { e.preventDefault(); void save() }} className="mb-6 space-y-4 border-y border-border bg-card py-5">
          <div className="grid grid-cols-2 gap-3"><label className="text-xs font-semibold text-text2">日期<input type="date" required value={form.meal_date} max={today()} onChange={(e) => setForm({ ...form, meal_date: e.target.value })} className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text" /></label><label className="text-xs font-semibold text-text2">餐次<select value={form.meal_type} onChange={(e) => setForm({ ...form, meal_type: e.target.value })} className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text">{meals.map((meal) => <option key={meal.value} value={meal.value}>{meal.label}</option>)}</select></label></div>
          <label className="block text-xs font-semibold text-text2">吃了什么<input required maxLength={100} value={form.dish_name} onChange={(e) => setForm({ ...form, dish_name: e.target.value, nutrition_mode: "clear" })} placeholder="例如：番茄鸡蛋面" className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text outline-none focus:border-primary" /></label>
          <label className="block text-xs font-semibold text-text2">我吃了多少（可选）<input maxLength={100} value={form.portion || ""} onChange={(e) => setForm({ ...form, portion: e.target.value, nutrition_mode: "clear" })} placeholder="例如半碗、2 个、约 150 克；不知道可留空" className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text" /></label>
          <label className="block text-xs font-semibold text-text2">菜系（可选）<input maxLength={40} list="cuisine-options" value={form.cuisine} onChange={(e) => setForm({ ...form, cuisine: e.target.value })} placeholder="自己填写或选择" className="mt-1 h-11 w-full rounded-lg border border-border bg-bg px-3 text-sm text-text outline-none focus:border-primary" /><datalist id="cuisine-options">{cuisines.map((name) => <option key={name} value={name} />)}</datalist></label>
          <NutritionEntryFields key={form.request_key} value={form} onChange={setForm} onBusyChange={setNutritionBusy} />
          <fieldset><legend className="mb-2 text-xs font-semibold text-text2">包含哪些食物类别（可选）</legend><div className="flex flex-wrap gap-2">{groups.map((group) => { const selected = form.food_groups.includes(group.value); return <label key={group.value} className={`cursor-pointer rounded-lg border px-3 py-2 text-xs font-medium ${selected ? "border-mint bg-mint-light text-mint" : "border-border bg-bg text-text2"}`}><input type="checkbox" checked={selected} onChange={() => setForm({ ...form, food_groups: selected ? form.food_groups.filter((value) => value !== group.value) : [...form.food_groups, group.value] })} className="sr-only" />{group.label}</label> })}</div></fieldset>
          <label className="block text-xs font-semibold text-text2">备注（可选）<textarea rows={2} maxLength={500} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} className="mt-1 w-full resize-none rounded-lg border border-border bg-bg p-3 text-sm text-text" /></label>
          {form.linked_record_id && <p className="text-xs text-text2">已关联菜谱记录 <button type="button" className="min-h-11 underline" onClick={() => setForm({ ...form, linked_record_id: null })}>解除关联</button></p>}
          <div className="flex gap-2"><button type="button" onClick={() => setShowForm(false)} className="h-11 flex-1 rounded-lg border border-border text-sm font-semibold">取消</button><button disabled={!form.dish_name.trim() || busy || nutritionBusy} className="h-11 flex-1 rounded-lg bg-primary text-sm font-semibold text-white disabled:opacity-50">{busy ? "保存中…" : editId ? "保存修改" : "保存记录"}</button></div>
        </form>}
        <div className="mb-4 flex items-center justify-between"><h2 className="text-base font-bold">手动记录</h2><button onClick={() => navigate("/history")} className="flex items-center gap-1 text-xs font-semibold text-primary">查看菜谱用餐记录 <ArrowRight size={14} /></button></div>
        {journalError ? <RequestState error={journalError} onRetry={() => { void reloadJournal() }} compact /> : journalLoading ? <div className="skeleton h-24 rounded-lg" /> : journal.length === 0 ? <p className="py-8 text-center text-sm text-text3">还没有手动记录，今天吃了什么？</p> : <div className="divide-y divide-border border-y border-border">{journal.map((entry) => <div key={entry.id} className="flex gap-3 py-3"><div className="min-w-0 flex-1"><div className="text-xs text-text3">{entry.meal_date} · {meals.find((meal) => meal.value === entry.meal_type)?.label}</div><div className="mt-0.5 text-sm font-semibold">{entry.dish_name}</div><div className="mt-1 text-xs text-text2">{[entry.cuisine, ...entry.food_groups.map((value) => groups.find((group) => group.value === value)?.label || value)].filter(Boolean).join(" · ")}</div><p className="mt-1 text-xs text-text2">{entry.portion || "份量未记录"}</p><div className="flex flex-wrap gap-3"><button className="min-h-11 text-xs font-semibold text-primary" onClick={() => edit(entry)}>编辑</button><button className="min-h-11 text-xs font-semibold text-primary" onClick={() => edit(entry, true)}>今天也吃了</button></div>{entry.notes && <p className="mt-1 text-xs text-text3">{entry.notes}</p>}</div><button onClick={() => void remove(entry.id)} title="删除记录" aria-label={`删除${entry.dish_name}`} className="self-start p-2 text-text3"><Trash2 size={16} /></button></div>)}</div>}
      </div> : <div>
        <div className="mb-5 inline-flex rounded-lg border border-border bg-card p-1" role="group" aria-label="报告时间范围">{([7, 30] as const).map((value) => <button key={value} onClick={() => setDays(value)} className={`h-9 min-w-20 rounded-md px-3 text-xs font-semibold ${days === value ? "bg-primary text-white" : "text-text3"}`}>近 {value} 天</button>)}</div>
        {reportError ? <RequestState error={reportError} onRetry={() => { void reloadReport() }} compact /> : reportLoading ? <div className="skeleton h-40 rounded-lg" /> : report && <HealthReportPanel key={userId} report={report} onRecord={(date) => { setEditId(null); setForm({ ...empty(), meal_date: date }); setView("journal"); setShowForm(true) }} />}
      </div>}
    </main>
  </div>
}
