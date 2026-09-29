import NutritionCatalogSearch from "./NutritionCatalogSearch"
import NutritionRecipeEditor from "./NutritionRecipeEditor"
import { useEffect, useState } from "react"
import type { Dispatch, SetStateAction } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import toast from "react-hot-toast"
import { healthApi, errorMessage } from "@/api"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import type { FoodJournalInput, NutrientValues, NutritionFood } from "@/types"

const fields: { key: keyof NutrientValues; label: string; unit: string }[] = [
  { key: "energy_kcal", label: "能量", unit: "kcal" }, { key: "protein_g", label: "蛋白质", unit: "g" },
  { key: "carbohydrate_g", label: "碳水化合物", unit: "g" }, { key: "fat_g", label: "脂肪", unit: "g" },
  { key: "fiber_g", label: "膳食纤维", unit: "g" }, { key: "sodium_mg", label: "钠", unit: "mg" },
]
const states: Record<string, string> = { as_sold: "包装出售状态", ready_to_eat: "即食", raw: "生", cooked: "熟" }
const blank = () => ({ name: "", basis_unit: "g" as "g" | "ml", food_state: "as_sold", source_reference: "", version: 1, nutrients: { energy_kcal: null, protein_g: null, carbohydrate_g: null, fat_g: null, fiber_g: null, sodium_mg: null } as NutrientValues })
const input = "mt-1 min-h-11 w-full min-w-0 rounded-xl border border-border bg-bg px-3 text-sm"
const button = "min-h-11 rounded-full border border-border px-4 text-xs font-semibold disabled:opacity-50"

export default function NutritionEntryFields({ value, onChange, onBusyChange }: { value: FoodJournalInput; onChange: Dispatch<SetStateAction<FoodJournalInput>>; onBusyChange: (busy: boolean) => void }) {
  const { data: foods = [], error, refetch } = useQuery({ queryKey: ["nutrition-foods"], queryFn: healthApi.foods })
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [recipeOpen, setRecipeOpen] = useState(false)
  const [catalogOpen, setCatalogOpen] = useState(false)
  const [recipeEditing, setRecipeEditing] = useState<NutritionFood | undefined>()
  const [draft, setDraft] = useState(blank)
  const [editing, setEditing] = useState<number | undefined>()
  const [busy, setBusy] = useState(false)
  const [energyUnit, setEnergyUnit] = useState("kcal")
  useEffect(() => { onBusyChange(open || recipeOpen || catalogOpen || busy); return () => onBusyChange(false) }, [open, recipeOpen, catalogOpen, busy, onBusyChange])
  async function deleteLabel(id = editing) {
    if (!id || busy || !window.confirm("删除此标签？已经保存的摄入快照仍保留。")) return
    const epoch = getSessionEpoch(); setBusy(true)
    try { await healthApi.deleteFood(id); if (!isCurrentSession(epoch)) return; await qc.invalidateQueries({ queryKey: ["nutrition-foods"] }); if (!isCurrentSession(epoch)) return; setOpen(false); onChange(current => ({ ...current, nutrition_mode: "clear", nutrition_food_id: undefined, nutrition_amount: undefined, nutrition_portion_key: undefined, nutrition_portion_count: undefined })); toast.success("标签已删除") }
    catch (error) { if (isCurrentSession(epoch)) toast.error(errorMessage(error)) }
    finally { if (isCurrentSession(epoch)) setBusy(false) }
  }
  function select(food: NutritionFood) {
    onChange(current => ({ ...current, nutrition_mode: "replace", nutrition_food_id: food.id, nutrition_unit: food.basis_unit, food_state: food.food_state, portion_source: food.recipe ? "estimated" : "measured", nutrition_amount: undefined, nutrition_portion_key: undefined, nutrition_portion_count: undefined }))
  }
  async function saveLabel() {
    if (busy) return
    const epoch = getSessionEpoch()
    setBusy(true)
    try {
      const nutrients = { ...draft.nutrients }
      if (energyUnit === "kJ" && nutrients.energy_kcal !== null) nutrients.energy_kcal /= 4.184
      const food = await healthApi.saveFood({ ...draft, nutrients }, editing)
      if (!isCurrentSession(epoch)) return
      await qc.invalidateQueries({ queryKey: ["nutrition-foods"] })
      if (!isCurrentSession(epoch)) return
      setOpen(false); select(food); toast.success("标签已保存，请填写实际食用量")
    } catch (e) { if (isCurrentSession(epoch)) toast.error(errorMessage(e)) }
    finally { if (isCurrentSession(epoch)) setBusy(false) }
  }
  const selected = foods.find(f => f.id === value.nutrition_food_id)
  const standardPortion = selected?.portions?.find(p => p.key === value.nutrition_portion_key)
  const keep = value.nutrition && value.nutrition_mode !== "clear" && value.nutrition_mode !== "replace"
  return <fieldset className="space-y-3 rounded-2xl border border-border p-4">
    <legend className="px-2 text-sm font-semibold">营养标签、配方与食用量（可选）</legend>
    <p className="text-xs leading-5 text-text2">只填写包装上标明的数值。标签由你录入、未经平台核验；没有标签可跳过，未知不会算成零。</p>
    {keep && <div className="text-xs leading-5 text-text2">保留已记录的 {value.nutrition?.food_name} · {value.nutrition?.amount} {value.nutrition?.unit}（来源版本 {value.nutrition?.food_version}）。修改标签不会改变这份历史快照。</div>}
    {error && <button type="button" className={button} onClick={() => void refetch()}>标签加载失败，点击重试</button>}
    <label className="block text-xs">选择我的标签或混合配方<select disabled={open || recipeOpen || catalogOpen || busy} value={value.nutrition_mode === "replace" ? value.nutrition_food_id || "" : ""} className={input} onChange={e => {
      const food = foods.find(f => f.id === Number(e.target.value))
      if (food) select(food); else onChange({ ...value, nutrition_mode: "clear", nutrition_food_id: undefined, nutrition_amount: undefined, nutrition_portion_key: undefined, nutrition_portion_count: undefined })
    }}><option value="">{keep ? "保留历史快照，或选择标签重新计算" : "暂不记录营养"}</option>{foods.map(f => <option key={f.id} value={f.id}>{f.name}{f.recipe ? "（混合配方）" : f.catalog ? "（标准食物）" : ""} · 每 100{f.basis_unit} · {states[f.food_state]}</option>)}</select></label>
    {selected && value.nutrition_mode === "replace" && <>
      <p className="text-xs leading-5 text-text2">来源：{selected.source_reference}。实际食物须与“{states[selected.food_state]}”一致；生熟或克/毫升不自动换算。</p>
      {!!selected.portions?.length && <label className="block text-xs">食用量填写方式<select className={input} value={value.nutrition_portion_key || ""} onChange={e => onChange({ ...value, nutrition_portion_key: e.target.value || undefined, nutrition_portion_count: undefined, nutrition_amount: undefined, portion_source: e.target.value ? "estimated" : "measured" })}>
        <option value="">直接填写克重 / 毫升</option>{selected.portions.map(p => <option key={p.key} value={p.key}>{p.label}（约 {p.amount} {selected.basis_unit}）</option>)}
      </select></label>}
      {standardPortion && <p className="text-xs leading-5 text-text2">每{standardPortion.label}约 {standardPortion.amount} {selected.basis_unit} 可食部分。依据：{standardPortion.reference}。按标准份量估算，实际大小可能不同。</p>}
      <div className="grid grid-cols-2 gap-3">
        {standardPortion ? <label className="text-xs">吃了几份“{standardPortion.label}”<input type="number" min="0.01" max="100" step="any" value={value.nutrition_portion_count ?? ""} className={input} onChange={e => onChange({ ...value, nutrition_portion_count: e.target.value === "" ? undefined : Number(e.target.value) })} /></label> : <label className="text-xs">我实际吃了多少（{selected.basis_unit}）<input type="number" min="0.01" max="10000" step="any" value={value.nutrition_amount ?? ""} className={input} onChange={e => onChange({ ...value, nutrition_amount: e.target.value === "" ? undefined : Number(e.target.value) })} /></label>}
        <label className="text-xs">份量依据<select disabled={!!selected.recipe || !!standardPortion} className={input} value={value.portion_source || "measured"} onChange={e => onChange({ ...value, portion_source: e.target.value })}><option value="measured">称量 / 量取</option><option value="estimated">估计份量</option></select></label>
      </div>
      <button type="button" disabled={open || recipeOpen || catalogOpen || busy || !!selected.catalog} className={button} onClick={() => { if (selected.recipe) { setRecipeEditing(selected); setRecipeOpen(true); return } setDraft({ ...selected }); setEditing(selected.id); setEnergyUnit("kcal"); setOpen(true) }}>{selected.recipe ? "修正配方" : selected.catalog ? "目录数据只读" : "修正这个标签"}</button>
    </>}
    {selected?.catalog && value.nutrition_mode === "replace" && <div className="text-xs leading-5 text-text2"><p>按可食部分称量。来源：{selected.catalog.dataset} · {selected.catalog.version} · {selected.catalog.record_id}；核验：{selected.catalog.reviewed_by}（{selected.catalog.reviewed_at}）。</p><p>许可：{selected.catalog.license}。目录更新不会自动替换此副本。</p><button type="button" disabled={open || recipeOpen || catalogOpen || busy} className={button} onClick={() => void deleteLabel(selected.id)}>从我的食物移除</button></div>}
    <div className="flex flex-wrap gap-2">
      <button type="button" disabled={open || recipeOpen || catalogOpen || busy} className={button} onClick={() => { setDraft(blank()); setEditing(undefined); setEnergyUnit("kcal"); setOpen(true) }}>录入包装标签</button>
      <button type="button" disabled={open || recipeOpen || catalogOpen || busy} className={button} onClick={() => { setRecipeEditing(undefined); setRecipeOpen(true) }}>新建混合配方</button>
      <button type="button" disabled={open || recipeOpen || catalogOpen || busy} className={button} onClick={() => setCatalogOpen(true)}>查找标准食物</button>
      {(keep || value.nutrition_mode === "replace") && <button type="button" className={button} onClick={() => onChange({ ...value, nutrition_mode: "clear", nutrition_food_id: undefined, nutrition_amount: undefined, nutrition_portion_key: undefined, nutrition_portion_count: undefined })}>清除本条营养快照</button>}
    </div>
    {selected?.recipe && value.nutrition_mode === "replace" && <div className="text-xs leading-5 text-text2">整份成品 {selected.recipe.yield_g} g，按均匀混合分摊估算。{selected.recipe.ingredients.map(i => <p key={i.food_id}>{i.food_name} · {i.amount} {i.unit} · 标签 v{i.food_version}</p>)}</div>}
    {catalogOpen && <NutritionCatalogSearch onClose={() => setCatalogOpen(false)} onAdopted={food => { qc.setQueryData<NutritionFood[]>(["nutrition-foods"], current => [...(current || []).filter(f => f.id !== food.id), food]); setCatalogOpen(false); select(food); toast.success("已添加，请填写可食部分的实际食用量") }} />}
    {recipeOpen && <NutritionRecipeEditor foods={foods} existing={recipeEditing} onDeleted={id => { qc.setQueryData<NutritionFood[]>(["nutrition-foods"], current => (current || []).filter(f => f.id !== id)); setRecipeOpen(false); onChange(current => ({ ...current, nutrition_mode: "clear", nutrition_food_id: undefined, nutrition_amount: undefined, nutrition_portion_key: undefined, nutrition_portion_count: undefined })); toast.success("配方已删除") }} onClose={() => setRecipeOpen(false)} onSaved={food => { qc.setQueryData<NutritionFood[]>(["nutrition-foods"], current => [...(current || []).filter(f => f.id !== food.id), food]); setRecipeOpen(false); select(food); toast.success("配方已保存，请填写实际食用量") }} />}
    {open && <div className="space-y-3 border-t border-border pt-4">
      <h3 className="text-sm font-bold">{editing ? "修正营养标签" : "我的包装标签"}</h3>
      <label className="block text-xs">食物名称<input maxLength={100} value={draft.name} className={input} onChange={e => setDraft({ ...draft, name: e.target.value })} /></label>
      <label className="block text-xs">来源说明<input maxLength={500} value={draft.source_reference} placeholder="品牌、包装规格、标签日期等" className={input} onChange={e => setDraft({ ...draft, source_reference: e.target.value })} /></label>
      <div className="grid grid-cols-2 gap-3">
        <label className="text-xs">标签基准<select className={input} value={draft.basis_unit} onChange={e => setDraft({ ...draft, basis_unit: e.target.value as "g" | "ml" })}><option value="g">每 100 克</option><option value="ml">每 100 毫升</option></select></label>
        <label className="text-xs">食物状态<select className={input} value={draft.food_state} onChange={e => setDraft({ ...draft, food_state: e.target.value })}>{Object.entries(states).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label>
      </div>
      <p className="text-xs leading-5 text-text2">若包装写“每份”，请先按包装净含量换算为每 100 克/毫升；无法确认时留空。</p>
      <label className="block text-xs">包装上的能量单位<select value={energyUnit} className={input} onChange={e => { setEnergyUnit(e.target.value); setDraft({ ...draft, nutrients: { ...draft.nutrients, energy_kcal: null } }) }}><option value="kcal">千卡 kcal</option><option value="kJ">千焦 kJ（保存时除以 4.184）</option></select></label>
      <div className="grid grid-cols-2 gap-3">{fields.map(f => <label key={f.key} className="text-xs">{f.label}（{f.key === "energy_kcal" ? energyUnit : f.unit}）<input type="number" min="0" step="any" placeholder="未知留空" className={input} value={draft.nutrients[f.key] ?? ""} onChange={e => setDraft({ ...draft, nutrients: { ...draft.nutrients, [f.key]: e.target.value === "" ? null : Number(e.target.value) } })} /></label>)}</div>
      <div className="flex flex-wrap gap-2"><button type="button" disabled={busy} className={button} onClick={() => void saveLabel()}>{busy ? "保存中…" : "保存标签"}</button><button type="button" disabled={busy} className={button} onClick={() => setOpen(false)}>取消</button>{editing && <button type="button" disabled={busy} className={button} onClick={() => void deleteLabel()}>删除标签</button>}</div>
    </div>}
  </fieldset>
}
