import { useEffect, useRef, useState } from "react"
import toast from "react-hot-toast"
import { healthApi, errorMessage } from "@/api"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import type { NutritionFood } from "@/types"

const input = "mt-1 min-h-11 w-full min-w-0 rounded-xl border border-border bg-bg px-3 text-sm"
const button = "min-h-11 rounded-full border border-border px-4 text-xs font-semibold disabled:opacity-50"

export default function NutritionRecipeEditor({ foods, existing, onSaved, onDeleted, onClose }: { foods: NutritionFood[]; existing?: NutritionFood; onSaved: (food: NutritionFood) => void; onDeleted: (id: number) => void; onClose: () => void }) {
  const [name, setName] = useState(existing?.name || "")
  const [reference, setReference] = useState(existing?.source_reference || "")
  const [yieldG, setYieldG] = useState(existing?.recipe?.yield_g.toString() || "")
  const [confirmed, setConfirmed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [rows, setRows] = useState(() => existing?.recipe?.ingredients.map(i => ({ foodId: i.food_id, amount: String(i.amount) })) || [{ foodId: 0, amount: "" }, { foodId: 0, amount: "" }])
  const alive = useRef(true)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const labels = foods.filter(f => f.source === "package_label" || f.source === "standard_food")
  async function save() {
    if (busy || !confirmed) return
    const ingredients = rows.map(row => { const food = labels.find(f => f.id === row.foodId); return { food_id: row.foodId, food_version: food?.version || 0, amount: Number(row.amount), unit: food?.basis_unit || "", food_state: food?.food_state || "" } })
    if (ingredients.some(i => !i.food_id || !Number.isFinite(i.amount) || i.amount <= 0) || !Number.isFinite(Number(yieldG)) || Number(yieldG) <= 0) { toast.error("请选择每项原料并填写有效用量和成品重量"); return }
    const epoch = getSessionEpoch(); setBusy(true)
    try {
      const food = await healthApi.saveRecipe({ name, source_reference: reference, version: existing?.version || 1, yield_g: Number(yieldG), method: "unheated_all_retained", ingredients }, existing?.id)
      if (alive.current && isCurrentSession(epoch)) onSaved(food)
    } catch (e) { if (alive.current && isCurrentSession(epoch)) toast.error(errorMessage(e)) }
    finally { if (alive.current && isCurrentSession(epoch)) setBusy(false) }
  }
  async function remove() {
    if (!existing || busy || !window.confirm("删除配方？历史摄入快照仍保留。")) return
    const epoch = getSessionEpoch(); setBusy(true)
    try { await healthApi.deleteFood(existing.id); if (alive.current && isCurrentSession(epoch)) onDeleted(existing.id) }
    catch (e) { if (alive.current && isCurrentSession(epoch)) toast.error(errorMessage(e)) }
    finally { if (alive.current && isCurrentSession(epoch)) setBusy(false) }
  }
  return <fieldset disabled={busy} className="space-y-3 border-t border-border pt-4">
    <h3 className="text-sm font-bold">{existing ? "修正混合配方" : "新建混合配方"}</h3>
    <p className="text-xs leading-5 text-text2">适合直接混合的酸奶麦片等。先录入所有原料的标签或标准食物；不适用于加热、弃汤、沥水或未计入调料的食物。每次修正会采用当前标签版本，历史记录不变。</p>
    <label className="block text-xs">配方名称<input className={input} maxLength={100} value={name} onChange={e => setName(e.target.value)} /></label>
    <label className="block text-xs">来源与制作说明<input className={input} maxLength={500} value={reference} onChange={e => setReference(e.target.value)} /></label>
    {rows.map((row, index) => {
      const food = labels.find(f => f.id === row.foodId)
      return <div key={index} className="space-y-2 rounded-xl border border-border p-3">
        <label className="block text-xs">原料 {index + 1}<select className={input} value={food ? row.foodId : ""} onChange={e => setRows(current => current.map((r, i) => i === index ? { foodId: Number(e.target.value), amount: "" } : r))}><option value="">{row.foodId ? "原标签已不可用，请重新选择" : "选择标签或标准食物"}</option>{labels.map(f => <option key={f.id} value={f.id}>{f.name} · {f.basis_unit} · {{ as_sold: "包装出售状态", ready_to_eat: "即食", raw: "生", cooked: "熟" }[f.food_state]} · v{f.version}</option>)}</select></label>
        <label className="block text-xs">实际加入量（{food?.basis_unit || "先选标签"}）<input className={input} type="number" min="0.01" max="10000" step="any" value={row.amount} onChange={e => setRows(current => current.map((r, i) => i === index ? { ...r, amount: e.target.value } : r))} /></label>
        <button type="button" className={button} disabled={rows.length <= 2} onClick={() => setRows(current => current.filter((_, i) => i !== index))}>移除原料</button>
      </div>
    })}
    <button type="button" className={button} disabled={rows.length >= 20} onClick={() => setRows(current => [...current, { foodId: 0, amount: "" }])}>添加原料</button>
    <label className="block text-xs">混合后整份成品重量（克）<input className={input} type="number" min="0.01" max="10000" step="any" value={yieldG} onChange={e => setYieldG(e.target.value)} /></label>
    <label className="flex min-h-11 items-start gap-2 text-xs leading-5"><input type="checkbox" className="mt-1" checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />我已列出全部原料，制作过程无需加热、没有弃去固体或液体，接受按均匀混合估算。</label>
    <p className="text-xs leading-5 text-text2">任何原料缺少某项营养值，整份配方的该项也保持未知。保存后再填写自己实际吃的成品克重。</p>
    <div className="flex flex-wrap gap-2"><button type="button" className={button} disabled={!confirmed || busy} onClick={() => void save()}>{busy ? "保存中…" : "保存配方"}</button><button type="button" className={button} onClick={onClose}>取消</button>{existing && <button type="button" className={button} onClick={() => void remove()}>删除配方</button>}</div>
  </fieldset>
}
