import { useEffect, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { ApiError, errorMessage, healthApi } from "@/api"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import { healthDate } from "@/lib/health-date"
import type { HealthDraftStatus, HealthMealDraftItem, HealthJournalBatchInput } from "@/types"

const button = "min-h-11 rounded-xl border border-border px-4 text-sm font-semibold disabled:opacity-50"
const field = "mt-1 min-h-11 w-full rounded-xl border border-border bg-bg px-3 text-sm text-text"
const meals = [{ value: "breakfast", label: "早餐" }, { value: "lunch", label: "午餐" }, { value: "dinner", label: "晚餐" }, { value: "snack", label: "加餐" }]

export default function HealthMealDraftPanel() {
  const qc = useQueryClient()
  const [owner] = useState(getSessionEpoch)
  const [open, setOpen] = useState(false)
  const [status, setStatus] = useState<HealthDraftStatus | null>(null)
  const [text, setText] = useState("")
  const [items, setItems] = useState<HealthMealDraftItem[] | null>(null)
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
  const submission = useRef<HealthJournalBatchInput | null>(null)
  useEffect(() => () => { request.current++; statusRequest.current++; abort.current?.abort() }, [])
  const current = (id: number) => id === request.current && isCurrentSession(owner)
  async function loadStatus() {
    const id = ++statusRequest.current
    try { const next = await healthApi.draftStatus(); if (id === statusRequest.current && isCurrentSession(owner)) setStatus(next) }
    catch { if (id === statusRequest.current && isCurrentSession(owner)) setStatus(null) }
  }
  function toggle() {
    if (saving || !isCurrentSession(owner)) return
    if (open) { request.current++; statusRequest.current++; abort.current?.abort(); setBusy(false) }
    else void loadStatus()
    setOpen(!open)
  }
  async function parse() {
    if (busy || submitted || !text.trim() || !isCurrentSession(owner)) return
    const id = ++request.current
    const controller = new AbortController(); abort.current = controller
    setBusy(true); setMessage(""); setItems(null); setConfirmed(false)
    try {
      const draft = await healthApi.parseDraft(text, controller.signal)
      if (!current(id)) return
      setItems(draft.items)
      if (!draft.items.length) setMessage("未找到可确认的一次实际饮食，请只描述自己已吃过的一餐，或使用手动记录。")
    } catch (error) { if (current(id)) setMessage(errorMessage(error)) }
    finally { if (current(id)) { setBusy(false); void loadStatus() } }
  }
  async function save() {
    if (saving || !isCurrentSession(owner) || !items?.length || !date || !meal || !confirmed) return
    if (!submission.current) submission.current = { request_key: crypto.randomUUID(), confirmed: true, meal_date: date, meal_type: meal, items: items.map(({ dish_name, portion }) => ({ dish_name, portion })) }
    const id = ++request.current
    setSaving(true); setSubmitted(true); setMessage("")
    try {
      await healthApi.confirmDraft(submission.current)
      if (!current(id)) return
      submission.current = null; setSubmitted(false); setItems(null); setText(""); setConfirmed(false); setMeal("")
      setMessage("已保存为饮食日记；需要营养数据时，可在日记中编辑并填写食物来源与实际用量。")
      await Promise.all(["food-journal", "health-report"].map(key => qc.invalidateQueries({ queryKey: [key] })))
    } catch (error) {
      if (current(id)) {
        if (error instanceof ApiError && error.status === 400) { submission.current = null; setSubmitted(false); setConfirmed(false); setMessage(errorMessage(error)) }
        else setMessage(`${errorMessage(error)}。可重试确认；同一份提交不会重复保存。`)
      }
    }
    finally { if (current(id)) setSaving(false) }
  }
  function update(index: number, patch: Partial<HealthMealDraftItem>) { setItems(items!.map((item, i) => i === index ? { ...item, ...patch } : item)); setConfirmed(false) }
  return <section className="mb-5 rounded-2xl border border-border bg-card p-4" aria-label="文字记餐草稿">
    <div className="flex items-center justify-between gap-3"><h2 className="text-sm font-bold">用一句话记餐</h2><button className={button} disabled={saving} onClick={toggle} aria-expanded={open}>{open ? "收起" : "打开文字记餐"}</button></div>
    {open && <div className="mt-3 space-y-3">
      <p className="text-xs leading-5 text-text2">仅把这段描述发送给已配置的 AI 整理，不读取你的健康档案。草稿需你核对后保存，份量未知可留空，不自动计算营养。</p>
      <p className="text-xs text-text2">{status ? status.enabled ? `今日可整理 ${status.quota.remaining}/${status.quota.limit} 次；请求发起后计次，失败也可能消耗额度。` : "文字整理尚未启用，可继续手动记餐。" : "正在读取服务状态；读取失败可重试。"}</p>
      {!status && <button className={button} onClick={() => void loadStatus()}>重试状态</button>}
      <label className="block text-xs font-semibold">描述自己实际吃过的一餐<textarea className={`${field} min-h-28 py-3`} maxLength={1000} disabled={busy || submitted} value={text} onChange={e => { setText(e.target.value); setItems(null); setConfirmed(false) }} placeholder="例如：中午我吃了番茄鸡蛋和半碗米饭" /></label>
      <div className="flex flex-wrap gap-2"><button className={button} disabled={busy || submitted || !text.trim() || !status?.enabled || !status.quota.remaining} onClick={() => void parse()}>{busy ? "正在整理…" : "整理成草稿"}</button>{busy && <button className={button} onClick={() => { request.current++; abort.current?.abort(); setBusy(false); setMessage("已停止整理，描述已保留。"); void loadStatus() }}>停止整理</button>}</div>
      {!!items?.length && <>
        <p className="text-sm font-semibold">待核对 · {items.length} 个食物项，尚未保存</p>
        <div className="grid grid-cols-2 gap-3"><label className="text-xs">实际日期<input className={field} type="date" max={healthDate()} disabled={submitted} value={date} onChange={e => { setDate(e.target.value); setConfirmed(false) }} /></label><label className="text-xs">实际餐次<select className={field} disabled={submitted} value={meal} onChange={e => { setMeal(e.target.value); setConfirmed(false) }}><option value="">请选择</option>{meals.map(m => <option key={m.value} value={m.value}>{m.label}</option>)}</select></label></div>
        {items.map((item, index) => <div key={index} className="space-y-2 border-t border-border pt-3">
          <p className="break-words text-xs leading-5 text-text2">原文：{item.evidence}</p>
          <label className="block text-xs">食物 {index + 1}<input className={field} maxLength={100} disabled={submitted} value={item.dish_name} onChange={e => update(index, { dish_name: e.target.value })} /></label>
          <label className="block text-xs">本人吃下的份量（可留空）<input className={field} maxLength={100} disabled={submitted} value={item.portion} onChange={e => update(index, { portion: e.target.value })} /></label>
          <button className={button} disabled={submitted} onClick={() => { setItems(items.filter((_, i) => i !== index)); setConfirmed(false) }}>移除食物 {index + 1}</button>
        </div>)}
        <label className="flex min-h-11 items-start gap-2 text-sm leading-6"><input className="mt-1.5" type="checkbox" disabled={submitted} checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />已核对同一次实际食用、日期和餐次；份量属于我本人，已移除未吃或重复项目。</label>
        <button className={button} disabled={saving || !confirmed || !meal || !date || items.some(i => !i.dish_name.trim())} onClick={() => void save()}>{saving ? "正在保存…" : submitted ? "重试确认保存" : "确认保存这餐"}</button>
        {submitted && <p className="text-xs text-text2">正在核对这次提交的保存结果，暂时保留同一份内容供重试。</p>}
      </>}
      {message && <p role="status" className="text-sm leading-6 text-text2">{message}</p>}
    </div>}
  </section>
}
