import { useEffect, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Search, Trash2, X } from "lucide-react"
import { ApiError, errorMessage, healthApi } from "@/api"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import { healthDate } from "@/lib/health-date"
import type { HealthDraftStatus, HealthMealDraftItem, HealthDraftCandidates, HealthDraftBatchItem, HealthJournalBatchInput, NutritionFood } from "@/types"

const button = "min-h-11 rounded-xl border border-border px-4 text-sm font-semibold disabled:opacity-50"
const field = "mt-1 min-h-11 w-full rounded-xl border border-border bg-bg px-3 text-sm text-text"
const meals = [{ value: "breakfast", label: "早餐" }, { value: "lunch", label: "午餐" }, { value: "dinner", label: "晚餐" }, { value: "snack", label: "加餐" }]
const states: Record<string, string> = { as_sold: "出售状态", ready_to_eat: "即食", raw: "生", cooked: "熟" }
type ReviewItem = HealthMealDraftItem & { food?: NutritionFood; amount: string; portionKey: string; portionCount: string; portionSource: "measured" | "estimated" }
const ready = (item: ReviewItem) => !item.food || (item.portionKey ? Number(item.portionCount) > 0 && Number(item.portionCount) <= 100 : Number(item.amount) > 0 && Number(item.amount) <= 10000)
function batchItem(item: ReviewItem): HealthDraftBatchItem {
  const base = { dish_name: item.dish_name.trim(), portion: item.portion.trim() }
  if (!item.food) return base
  const nutrition = { nutrition_mode: "replace" as const, nutrition_food_id: item.food.id, nutrition_unit: item.food.basis_unit, food_state: item.food.food_state, portion_source: item.portionKey ? "estimated" as const : item.portionSource }
  return item.portionKey ? { ...base, ...nutrition, nutrition_portion_key: item.portionKey, nutrition_portion_count: Number(item.portionCount) } : { ...base, ...nutrition, nutrition_amount: Number(item.amount) }
}

export default function HealthMealDraftPanel() {
  const qc = useQueryClient()
  const [owner] = useState(getSessionEpoch)
  const [open, setOpen] = useState(false)
  const [status, setStatus] = useState<HealthDraftStatus | null>(null)
  const [text, setText] = useState("")
  const [items, setItems] = useState<ReviewItem[] | null>(null)
  const [searchIndex, setSearchIndex] = useState<number | null>(null)
  const [searchQuery, setSearchQuery] = useState("")
  const [candidates, setCandidates] = useState<HealthDraftCandidates | null>(null)
  const [candidateBusy, setCandidateBusy] = useState(false)
  const [candidateMessage, setCandidateMessage] = useState("")
  const [date, setDate] = useState(healthDate)
  const [meal, setMeal] = useState("")
  const [confirmed, setConfirmed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [message, setMessage] = useState("")
  const request = useRef(0)
  const statusRequest = useRef(0)
  const abort = useRef<AbortController | null>(null)
  const candidateRequest = useRef(0)
  const candidateAbort = useRef<AbortController | null>(null)
  const submission = useRef<HealthJournalBatchInput | null>(null)
  useEffect(() => () => { request.current++; statusRequest.current++; candidateRequest.current++; abort.current?.abort(); candidateAbort.current?.abort() }, [])
  const current = (id: number) => id === request.current && isCurrentSession(owner)
  function closeCandidates() {
    candidateRequest.current++; candidateAbort.current?.abort()
    setSearchIndex(null); setCandidates(null); setCandidateBusy(false); setCandidateMessage("")
  }
  async function loadStatus() {
    const id = ++statusRequest.current
    try { const next = await healthApi.draftStatus(); if (id === statusRequest.current && isCurrentSession(owner)) setStatus(next) }
    catch { if (id === statusRequest.current && isCurrentSession(owner)) setStatus(null) }
  }
  function toggle() {
    if (saving || !isCurrentSession(owner)) return
    if (open) { request.current++; statusRequest.current++; abort.current?.abort(); setBusy(false); closeCandidates() }
    else void loadStatus()
    setOpen(!open)
  }
  async function parse() {
    if (busy || candidateBusy || submitted || !text.trim() || !isCurrentSession(owner)) return
    const id = ++request.current
    const controller = new AbortController(); abort.current = controller
    closeCandidates()
    setBusy(true); setMessage(""); setItems(null); setConfirmed(false)
    try {
      const draft = await healthApi.parseDraft(text, controller.signal)
      if (!current(id)) return
      setItems(draft.items.map(item => ({ ...item, amount: "", portionKey: "", portionCount: "", portionSource: "estimated" })))
      if (!draft.items.length) setMessage("未找到可确认的一次实际饮食，请只描述自己已吃过的一餐，或使用手动记录。")
    } catch (error) { if (current(id)) setMessage(errorMessage(error)) }
    finally { if (current(id)) { setBusy(false); void loadStatus() } }
  }
  async function save() {
    if (saving || candidateBusy || !isCurrentSession(owner) || !items?.length || !date || !meal || !confirmed || items.some(item => !ready(item))) return
    if (!submission.current) submission.current = { request_key: crypto.randomUUID(), confirmed: true, meal_date: date, meal_type: meal, items: items.map(batchItem) }
    const id = ++request.current
    closeCandidates()
    setSaving(true); setSubmitted(true); setMessage("")
    try {
      await healthApi.confirmDraft(submission.current)
      if (!current(id)) return
      submission.current = null; setSubmitted(false); setItems(null); setText(""); setConfirmed(false); setMeal("")
      setMessage("已保存为饮食日记。未选择营养来源的食物仍可在日记中补充。")
      await Promise.all(["food-journal", "health-report"].map(key => qc.invalidateQueries({ queryKey: [key] })))
    } catch (error) {
      if (current(id)) {
        if (error instanceof ApiError && error.status === 400) { submission.current = null; setSubmitted(false); setConfirmed(false); setMessage(errorMessage(error)) }
        else setMessage(`${errorMessage(error)}。可重试确认；同一份提交不会重复保存。`)
      }
    }
    finally { if (current(id)) setSaving(false) }
  }
  function update(index: number, patch: Partial<ReviewItem>) { setItems(current => current?.map((item, i) => i === index ? { ...item, ...patch } : item) ?? null); setConfirmed(false) }
  function selectFood(index: number, food: NutritionFood) {
    if (submitted || !items?.[index] || searchIndex !== index || !isCurrentSession(owner)) return
    update(index, { food, amount: "", portionKey: "", portionCount: "", portionSource: food.recipe ? "estimated" : "measured" })
    closeCandidates()
  }
  function openSearch(index: number) {
    if (submitted || !isCurrentSession(owner)) return
    candidateRequest.current++; candidateAbort.current?.abort()
    setCandidateBusy(false); setSearchIndex(index); setSearchQuery(items![index].dish_name); setCandidates(null); setCandidateMessage("")
  }
  async function findCandidates() {
    if (submitted || searchIndex === null || !searchQuery.trim() || candidateBusy || !isCurrentSession(owner)) return
    const id = ++candidateRequest.current
    const controller = new AbortController(); candidateAbort.current = controller
    setCandidateBusy(true); setCandidates(null); setCandidateMessage("")
    try {
      const result = await healthApi.draftCandidates(searchQuery.trim(), controller.signal)
      if (id !== candidateRequest.current || !isCurrentSession(owner)) return
      setCandidates(result)
      if (!result.personal.length && !result.catalog.length) setCandidateMessage("没有可用来源，可只保存文字，稍后在日记中补充。")
    } catch (error) { if (id === candidateRequest.current && isCurrentSession(owner)) setCandidateMessage(errorMessage(error)) }
    finally { if (id === candidateRequest.current && isCurrentSession(owner)) setCandidateBusy(false) }
  }
  async function adopt(index: number, id: number) {
    if (submitted || candidateBusy || !isCurrentSession(owner)) return
    const candidateId = candidateRequest.current
    setCandidateBusy(true); setCandidateMessage("")
    try {
      const food = await healthApi.adoptCatalog(id)
      if (candidateId === candidateRequest.current && isCurrentSession(owner) && searchIndex === index) selectFood(index, food)
    } catch (error) { if (candidateId === candidateRequest.current && isCurrentSession(owner)) setCandidateMessage(errorMessage(error)) }
    finally { if (candidateId === candidateRequest.current && isCurrentSession(owner)) setCandidateBusy(false) }
  }
  return <section className="mb-5 rounded-2xl border border-border bg-card p-4" aria-label="文字记餐草稿">
    <div className="flex items-center justify-between gap-3"><h2 className="text-sm font-bold">用一句话记餐</h2><button className={button} disabled={saving} onClick={toggle} aria-expanded={open}>{open ? "收起" : "打开文字记餐"}</button></div>
    {open && <div className="mt-3 space-y-3">
      <p className="text-xs leading-5 text-text2">仅把这段描述发送给已配置的 AI 整理，不读取你的健康档案。草稿需你核对后保存；未选择营养来源时不计算营养。</p>
      <p className="text-xs text-text2">{status ? status.enabled ? `今日可整理 ${status.quota.remaining}/${status.quota.limit} 次；请求发起后计次，失败也可能消耗额度。` : "文字整理尚未启用，可继续手动记餐。" : "正在读取服务状态；读取失败可重试。"}</p>
      {!status && <button className={button} onClick={() => void loadStatus()}>重试状态</button>}
      <label className="block text-xs font-semibold">描述自己实际吃过的一餐<textarea className={`${field} min-h-28 py-3`} maxLength={1000} disabled={busy || submitted} value={text} onChange={e => { closeCandidates(); setText(e.target.value); setItems(null); setConfirmed(false) }} placeholder="例如：中午我吃了番茄鸡蛋和半碗米饭" /></label>
      <div className="flex flex-wrap gap-2"><button className={button} disabled={busy || candidateBusy || submitted || !text.trim() || !status?.enabled || !status.quota.remaining} onClick={() => void parse()}>{busy ? "正在整理…" : "整理成草稿"}</button>{busy && <button className={button} onClick={() => { request.current++; abort.current?.abort(); setBusy(false); setMessage("已停止整理，描述已保留。"); void loadStatus() }}>停止整理</button>}</div>
      {!!items?.length && <>
        <p className="text-sm font-semibold">待核对 · {items.length} 个食物项，尚未保存</p>
        <div className="grid grid-cols-2 gap-3"><label className="text-xs">实际日期<input className={field} type="date" max={healthDate()} disabled={submitted} value={date} onChange={e => { setDate(e.target.value); setConfirmed(false) }} /></label><label className="text-xs">实际餐次<select className={field} disabled={submitted} value={meal} onChange={e => { setMeal(e.target.value); setConfirmed(false) }}><option value="">请选择</option>{meals.map(m => <option key={m.value} value={m.value}>{m.label}</option>)}</select></label></div>
        {items.map((item, index) => <div key={index} className="space-y-2 border-t border-border pt-3">
          <p className="break-words text-xs leading-5 text-text2">原文：{item.evidence}</p>
          <label className="block text-xs">食物 {index + 1}<input className={field} maxLength={100} disabled={submitted} value={item.dish_name} onChange={e => { update(index, { dish_name: e.target.value, food: undefined }); if (searchIndex === index) closeCandidates() }} /></label>
          <label className="block text-xs">本人吃下的份量（可留空）<input className={field} maxLength={100} disabled={submitted} value={item.portion} onChange={e => update(index, { portion: e.target.value })} /></label>
          <div className="flex flex-wrap gap-2">
            <button className={`${button} inline-flex items-center gap-2`} disabled={submitted} onClick={() => openSearch(index)}><Search size={16} />{item.food ? "更换营养来源" : "查找营养来源"}</button>
            <button className={`${button} inline-flex items-center gap-2`} disabled={submitted} onClick={() => { if (searchIndex !== null) closeCandidates(); setItems(items.filter((_, i) => i !== index)); setConfirmed(false) }}><Trash2 size={16} />移除</button>
          </div>
          {item.food && <div className="space-y-2 border-l-2 border-primary pl-3 text-xs">
            <div className="flex items-center justify-between gap-2"><span>{item.food.name} · {states[item.food.food_state] || item.food.food_state} · 每100{item.food.basis_unit} · {item.food.source === "package_label" ? "本人包装标签" : item.food.source === "standard_food" ? "已采纳目录" : "本人配方"}</span><button className="p-2" type="button" disabled={submitted} aria-label="清除营养来源" title="清除营养来源" onClick={() => update(index, { food: undefined, amount: "", portionKey: "", portionCount: "" })}><X size={16} /></button></div>
            {!!item.food.portions?.length && <label className="block">用量方式<select className={field} disabled={submitted} value={item.portionKey} onChange={e => update(index, { portionKey: e.target.value, portionCount: "", amount: "" })}><option value="">直接填写{item.food.basis_unit}</option>{item.food.portions.map(portion => <option key={portion.key} value={portion.key}>{portion.label} · 每份 {portion.amount}{item.food!.basis_unit}</option>)}</select></label>}
            {item.portionKey ? <label className="block">本人实际吃下的份数<input className={field} type="number" min="0.01" max="100" step="any" disabled={submitted} value={item.portionCount} onChange={e => update(index, { portionCount: e.target.value })} /></label> : <><label className="block">本人实际吃下的可食部分（{item.food.basis_unit}）<input className={field} type="number" min="0.01" max="10000" step="any" disabled={submitted} value={item.amount} onChange={e => update(index, { amount: e.target.value })} /></label>{!item.food.recipe && <label className="block">份量依据<select className={field} disabled={submitted} value={item.portionSource} onChange={e => update(index, { portionSource: e.target.value as "measured" | "estimated" })}><option value="measured">称量或量取</option><option value="estimated">估算</option></select></label>}</>}
            <p className="text-text2">{item.food.source_reference}</p>
          </div>}
          {searchIndex === index && <div className="space-y-2 border border-border bg-bg p-3 text-xs">
            <div className="flex gap-2"><input className={`${field} min-w-0 flex-1`} maxLength={100} disabled={submitted} value={searchQuery} onChange={e => { candidateRequest.current++; candidateAbort.current?.abort(); setCandidateBusy(false); setCandidates(null); setCandidateMessage(""); setSearchQuery(e.target.value) }} aria-label="搜索食物来源" /><button className={`${button} shrink-0`} disabled={submitted || candidateBusy || !searchQuery.trim()} onClick={() => void findCandidates()} aria-label="搜索候选" title="搜索候选"><Search size={18} /></button><button className={`${button} shrink-0`} title="关闭候选" aria-label="关闭候选" onClick={closeCandidates}><X size={18} /></button></div>
            {candidateBusy && <p>正在查找…</p>}{candidateMessage && <p role="status">{candidateMessage}</p>}
            {!!candidates?.personal.length && <div><p className="font-semibold">我的食物</p>{candidates.personal.map(food => <button key={food.id} disabled={submitted || candidateBusy} className="block min-h-11 w-full border-b border-border py-2 text-left disabled:opacity-50" onClick={() => selectFood(index, food)}>{food.name} · {states[food.food_state] || food.food_state} · 每100{food.basis_unit} · {food.source_reference}</button>)}</div>}
            {!!candidates?.catalog.length && <div><p className="font-semibold">标准目录</p>{candidates.catalog.map(food => <div key={food.id} className="border-b border-border py-2"><button disabled={submitted || candidateBusy} className="min-h-11 w-full text-left disabled:opacity-50" onClick={() => void adopt(index, food.id)}>加入我的食物并选择：{food.name} · {states[food.food_state] || food.food_state} · {food.provenance.dataset} {food.provenance.version}</button><p className="break-words text-text2">{food.provenance.license} · {food.provenance.reviewed_by} · {food.provenance.reviewed_at}</p><a className="break-all text-primary underline" href={food.provenance.url} target="_blank" rel="noreferrer">查看来源</a></div>)}</div>}
          </div>}
        </div>)}
        <label className="flex min-h-11 items-start gap-2 text-sm leading-6"><input className="mt-1.5" type="checkbox" disabled={submitted} checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />已核对同一次实际食用、日期和餐次；份量属于我本人，已移除未吃或重复项目。</label>
        <button className={button} disabled={saving || candidateBusy || !confirmed || !meal || !date || items.some(i => !i.dish_name.trim() || !ready(i))} onClick={() => void save()}>{saving ? "正在保存…" : submitted ? "重试确认保存" : "确认保存这餐"}</button>
        {submitted && <p className="text-xs text-text2">正在核对这次提交的保存结果，暂时保留同一份内容供重试。</p>}
      </>}
      {message && <p role="status" className="text-sm leading-6 text-text2">{message}</p>}
    </div>}
  </section>
}
