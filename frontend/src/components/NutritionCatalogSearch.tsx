import { useEffect, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import toast from "react-hot-toast"
import { errorMessage, healthApi } from "@/api"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import type { NutritionFood } from "@/types"

const states: Record<string, string> = { as_sold: "出售状态", ready_to_eat: "即食", raw: "生", cooked: "熟" }
const input = "mt-1 min-h-11 w-full min-w-0 rounded-xl border border-border bg-bg px-3 text-sm"
const button = "min-h-11 rounded-full border border-border px-4 text-xs font-semibold disabled:opacity-50"

export default function NutritionCatalogSearch({ onAdopted, onClose }: { onAdopted: (food: NutritionFood) => void; onClose: () => void }) {
  const [query, setQuery] = useState("")
  const [submitted, setSubmitted] = useState("")
  const [state, setState] = useState("")
  const [busy, setBusy] = useState(false)
  const alive = useRef(true)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const { data, isFetching, error, refetch } = useQuery({ queryKey: ["nutrition-catalog", submitted, state], queryFn: () => healthApi.catalog(submitted, state) })
  async function adopt(id: number) {
    if (busy) return
    const epoch = getSessionEpoch(); setBusy(true)
    try { const food = await healthApi.adoptCatalog(id); if (alive.current && isCurrentSession(epoch)) onAdopted(food) }
    catch (e) { if (alive.current && isCurrentSession(epoch)) toast.error(errorMessage(e)) }
    finally { if (alive.current && isCurrentSession(epoch)) setBusy(false) }
  }
  return <fieldset disabled={busy} className="space-y-3 border-t border-border pt-4">
    <h3 className="text-sm font-bold">查找有来源的标准食物</h3>
    <p className="text-xs leading-5 text-text2">选择与实际食物及生熟状态一致的条目。数值按可食部分每100克/毫升提供，不能直接用于带皮、带骨重量；目录核验不代表医学认证。</p>
    <label className="block text-xs">食物名称或别名<input className={input} maxLength={100} value={query} onChange={e => setQuery(e.target.value)} /></label>
    <label className="block text-xs">食物状态<select className={input} value={state} onChange={e => setState(e.target.value)}><option value="">所有状态</option>{Object.entries(states).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
    <div className="flex gap-2"><button type="button" className={button} onClick={() => { if (submitted === query.trim()) void refetch(); else setSubmitted(query.trim()) }}>搜索</button><button type="button" className={button} onClick={onClose}>返回录入</button></div>
    {isFetching ? <p className="text-xs text-text2" role="status">正在查询目录…</p> : error ? <button type="button" className={button} onClick={() => void refetch()}>目录加载失败，重试</button> : data?.length === 0 ? <p className="text-xs leading-5 text-text2">当前没有匹配且已核验的食物。可以调整名称/状态，或返回录入包装标签；未配置目录时仍可普通记餐。</p> : data?.map(food => <article key={food.id} className="space-y-2 rounded-xl border border-border p-3 text-xs leading-5">
      <h4 className="text-sm font-semibold">{food.name} · {states[food.food_state]}</h4>
      <p>每100{food.basis_unit} · 可食部分</p>
      <p className="break-words text-text2">来源：{food.provenance.dataset} · {food.provenance.version} · {food.provenance.record_id}</p>
      <p className="break-words text-text2">许可：{food.provenance.license}<br />核验：{food.provenance.reviewed_by} · {food.provenance.reviewed_at}</p>
      <a className="inline-block min-h-11 py-3 underline" href={food.provenance.url} target="_blank" rel="noreferrer">查看原始来源</a>
      <div><button type="button" className={button} onClick={() => void adopt(food.id)}>{busy ? "添加中…" : "添加到我的食物并选择"}</button></div>
    </article>)}
    {data?.length === 50 && <p className="text-xs text-text2">最多显示50项，请细化搜索词。</p>}
  </fieldset>
}
