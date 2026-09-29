import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import toast from "react-hot-toast"
import { healthApi, errorMessage } from "@/api"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import { healthDate } from "@/lib/health-date"
import type { CatalogProvenance, HealthReport } from "@/types"

const mealNames: Record<string, string> = { breakfast: "早餐", lunch: "午餐", dinner: "晚餐", snack: "加餐" }
const groupNames: Record<string, string> = { vegetable: "蔬菜", fruit: "水果", protein: "蛋白质食物", whole_grain: "全谷物", dairy: "奶类" }
const statusNames = { unknown: "未记录", partial: "待确认完整", complete: "已确认完整" }
const button = "min-h-11 rounded-full border border-border px-4 text-sm font-semibold disabled:opacity-50"
const section = "rounded-[28px] bg-card p-5 sm:p-6"


function CatalogEvidence({ catalog }: { catalog: CatalogProvenance }) {
  return <div className="mt-1 break-words text-xs leading-5 text-text2">
    <p>标准食物：{catalog.dataset} · {catalog.version} · {catalog.record_id}；按可食部分。核验：{catalog.reviewed_by}（{catalog.reviewed_at}）；许可：{catalog.license}。</p>
    <a href={catalog.url} target="_blank" rel="noreferrer" className="inline-block min-h-11 py-3 underline">查看原始来源</a>
  </div>
}

export default function HealthReportPanel({ report, onRecord }: { report: HealthReport; onRecord: (date: string) => void }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [selected, setSelected] = useState("")
  const [date, setDate] = useState(healthDate)
  const [meal, setMeal] = useState("dinner")
  const [busy, setBusy] = useState(false)
  const day = report.days.find(d => d.date === selected) || report.days[report.days.length - 1]
  async function act(work: () => Promise<unknown>, message: string) {
    if (busy) return
    const epoch = getSessionEpoch()
    setBusy(true)
    try {
      await work()
      if (!isCurrentSession(epoch)) return
      await Promise.all(["health-report", "food-journal", "week-plan", "shopping-list", "shopping-overview"].map(key => qc.invalidateQueries({ queryKey: [key] })))
      if (isCurrentSession(epoch)) toast.success(message)
    } catch (error) { if (isCurrentSession(epoch)) toast.error(errorMessage(error)) }
    finally { if (isCurrentSession(epoch)) setBusy(false) }
  }
  return <div className="space-y-5">
    <section className={section}>
      <p className="text-xs text-text2">{report.from} — {report.to} · 北京时间</p>
      <h2 className="mt-2 text-xl font-bold">先看记录，再看饮食</h2>
      <p className="mt-3 text-sm leading-6 text-text2">{report.logged_days} 天有记录，{report.complete_days} 天确认完整。共 {report.meal_event_count} 餐、{report.item_count} 个食物项。</p>
      {report.portion_coverage && <div className="mt-4 border-t border-border pt-3 text-xs leading-5 text-text2">
        <h3 className="font-semibold text-text">食用量的依据</h3>
        <p className="mt-1">称量 / 量取 {report.portion_coverage.measured_items} 项 · 标准份量估算 {report.portion_coverage.standard_items} 项 · 其他估算 {report.portion_coverage.estimated_items} 项</p>
        <p>仅文字份量 {report.portion_coverage.text_only_items} 项 · 份量未知 {report.portion_coverage.unknown_items} 项</p>
        <p className="mt-1">按去重食物项统计；文字份量不会自动换算，称量也不代表营养值经过核验。</p>
      </div>}
      <div className="mt-5 grid grid-cols-4 gap-2 sm:grid-cols-7" aria-label="每日记录状态">
        {report.days.map(d => <button key={d.date} onClick={() => setSelected(d.date)} aria-pressed={day?.date === d.date} className={`min-h-16 rounded-2xl border px-1 py-2 text-center ${day?.date === d.date ? "border-primary bg-primary-light" : "border-border"}`}>
          <span className="block text-sm font-semibold">{d.date.slice(5)}</span>
          <span className="mt-1 block text-[10px] text-text2">{d.status === "complete" ? "完整" : d.status === "partial" ? "待确认" : "未记录"}</span>
        </button>)}
      </div>
      {day && <div className="mt-5 border-t border-border pt-4">
        <h3 className="font-semibold">{day.date} · {statusNames[day.status]}</h3>
        <p className="mt-1 text-xs text-text2">{day.meal_event_count} 餐 · {day.item_count} 个食物项；原始来源 {day.meal_count} 条</p>
        <ul className="mt-3 divide-y divide-border">
          {day.evidence.map(e => <li key={`${e.source}-${e.id}`} className="py-3 text-sm">
            <div>{mealNames[e.meal_type]} · {e.dish_name}</div>
            <p className="mt-1 text-xs text-text2">{e.source === "record" ? "菜谱用餐记录" : "饮食日记"} · {e.portion || "份量未记录"}{e.linked_record_id ? " · 已关联同一食物，不重复计算" : ""}</p>
            {e.nutrition && <p className="mt-1 text-xs leading-5 text-text2">营养来源：{e.nutrition.source_reference} · {e.nutrition.amount} {e.nutrition.unit} · {e.nutrition.portion_source === "measured" ? "称量/量取" : "估计份量"} · 来源 v{e.nutrition.food_version}</p>}
            {e.nutrition?.standard_portion && <p className="mt-1 break-words text-xs leading-5 text-text2">标准份量估算：{e.nutrition.standard_portion.count} × {e.nutrition.standard_portion.label}（每份 {e.nutrition.standard_portion.amount} {e.nutrition.unit}）；依据：{e.nutrition.standard_portion.reference}。</p>}
            {e.nutrition?.catalog && <CatalogEvidence catalog={e.nutrition.catalog} />}
            {e.nutrition?.recipe && <details className="mt-2 text-xs leading-5 text-text2">
              <summary className="min-h-11 cursor-pointer py-3">混合配方依据 · 成品 {e.nutrition.recipe.yield_g} g</summary>
              <p>按均匀混合分摊估算，无需加热且全部原料保留。</p>
              {e.nutrition.recipe.ingredients.map(i => <div key={i.food_id} className="mt-2 break-words">
                <p>{i.food_name} · {i.amount} {i.unit} · 来源 v{i.food_version} · {i.source_reference}</p>
                {i.catalog && <CatalogEvidence catalog={i.catalog} />}
              </div>)}
            </details>}
            {e.possible_duplicate_ids.map(id => <button key={id} disabled={busy} className={`${button} mt-2`} onClick={() => {
              if (!window.confirm("确认这条日记和菜谱记录是同一次食用？关联后只计一个食物项，原始记录仍保留。")) return
              void act(async () => { const entry = await healthApi.entry(e.id); await healthApi.update(e.id, { ...entry, linked_record_id: id }) }, "已关联，同一食物只计算一次")
            }}>与菜谱记录相同？确认关联</button>)}
          </li>)}
        </ul>
        <div className="mt-3 flex flex-wrap gap-2">
          <button className={button} onClick={() => onRecord(day.date)}>补记这一天</button>
          {day.item_count > 0 && <button disabled={busy} className={button} onClick={() => {
            if (day.status !== "complete" && !window.confirm("已记录这一天吃过的食物、饮料和加餐？没有吃的餐次不需要补填。")) return
            void act(() => healthApi.confirmDay(day.date, day.fingerprint, day.status !== "complete"), "记录状态已更新")
          }}>{day.status === "complete" ? "改为未确认" : "确认当天已记完整"}</button>}
        </div>
      </div>}
    </section>
    <section className={section}><h2 className="font-bold">本期观察</h2>{report.insights.map(text => <p key={text} className="mt-3 text-sm leading-6 text-text2">{text}</p>)}</section>
    <section className={section}>
      <h2 className="font-bold">食物类别出现的日子</h2>
      <div className="mt-4 space-y-4">{Object.entries(groupNames).map(([key, name]) => <div key={key}>
        <div className="flex justify-between text-sm"><span>{name}</span><span>{report.food_group_days[key] || 0} 天</span></div>
        <div className="mt-2 h-2 overflow-hidden rounded-full bg-bg"><div className="h-full bg-primary" style={{ width: `${100 * (report.food_group_days[key] || 0) / report.period_days}%` }} /></div>
      </div>)}</div>
      <p className="mt-4 text-xs leading-5 text-text2">仅统计手动标注；未标注不代表没有吃。这是出现天数，不是摄入量。</p>
    </section>
    <section className={section}>
      <h2 className="font-bold">有来源的营养记录</h2>
      <p className="mt-3 text-xs leading-5 text-text2">根据个人标签、已选标准食物和食用量计算，包含份量估计；不是检测结果或个性化摄入目标。未知项不计为零。</p>
      <div className="mt-4 space-y-4">{(report.nutrients || []).map(n => <div key={n.code} className="border-t border-border pt-3">
        <div className="flex flex-wrap items-baseline justify-between gap-2"><span className="text-sm font-semibold">{n.label}</span><span className="text-sm">{n.known_total === null ? "未记录" : `已知部分合计 ${n.known_total} ${n.unit}`}</span></div>
        <p className="mt-1 text-xs leading-5 text-text2">覆盖 {n.covered_items}/{n.total_items} 个食物项。{n.daily_average === null ? `完整且数据齐全的日子 ${n.complete_days}/${n.required_days}，暂不显示日均值。` : `${n.complete_days} 个完整且数据齐全日的均值：${n.daily_average} ${n.unit}/天。`}</p>
      </div>)}</div>
      <p className="mt-4 text-xs leading-5 text-text2">日均值门槛为本期至少 4/7 的天数，属于记录覆盖规则。缺少食物或该营养素的日子不参与均值，不据此判断营养不足。</p>
    </section>
    {report.comparison && <section className={section}>
      <h2 className="font-bold">和上一周期比一比</h2>
      <p className="mt-3 text-xs leading-5 text-text2">本期 {report.from} — {report.to}<br />上期 {report.comparison.from} — {report.comparison.to}</p>
      <p className="mt-2 text-xs leading-5 text-text2">按相同星期几匹配记录完整、营养齐全的日子。可能包含份量估计，升降不表示健康改善或恶化。</p>
      {report.comparison.nutrients.map(n => <div key={n.code} className="mt-4 border-t border-border pt-3">
        <h3 className="text-sm font-semibold">{n.label} · 已匹配 {n.matched_days}/{n.required_days} 对日期</h3>
        {n.status === "available" && n.delta !== null ? <>
          <div className="mt-2 grid grid-cols-2 gap-3 text-sm tabular-nums"><p>上期配对日均<br />{n.previous_average} {n.unit}/天</p><p>本期配对日均<br />{n.current_average} {n.unit}/天</p></div>
          <p className="mt-2 text-sm">差值 {n.delta > 0 ? "+" : ""}{n.delta} {n.unit}/天</p>
        </> : null}
        <p className="mt-2 text-xs leading-5 text-text2">{n.reason}</p>
        {n.matched_days > 0 && <details className="mt-2 text-xs text-text2"><summary className="cursor-pointer py-2">查看配对日期</summary><ul className="space-y-1">{n.current_dates.map((date, index) => <li key={date}>{n.previous_dates[index]} → {date}</li>)}</ul></details>}
      </div>)}
    </section>}
    <section className={section}>
      <h2 className="font-bold">给下一餐一个小安排</h2>
      {report.plan_actions.map(text => <p key={text} className="mt-3 text-sm leading-6 text-text2">{text}</p>)}
      <div className="mt-4 grid grid-cols-2 gap-3">
        <label className="text-xs">安排日期<input aria-label="安排日期" type="date" min={healthDate()} max={healthDate(6)} value={date} onChange={e => setDate(e.target.value)} className="mt-1 min-h-11 w-full rounded-xl border border-border bg-bg px-2 text-sm" /></label>
        <label className="text-xs">餐次<select value={meal} onChange={e => setMeal(e.target.value)} className="mt-1 min-h-11 w-full rounded-xl border border-border bg-bg px-2 text-sm"><option value="lunch">午餐</option><option value="dinner">晚餐</option></select></label>
      </div>
      <p className="mt-3 text-xs leading-5 text-text2">加入后以你的选择替换该餐随机推荐，食材同步到买菜清单。今明两天的食材在现有清单页展示；实际吃过后再记餐。</p>
      {report.recommendations.length === 0 && <p className="mt-4 text-sm">暂无符合当前筛选条件的菜谱，可以补充适合自己的私房菜。不会为了凑数放宽档案中的过敏与禁忌。</p>}
      {report.recommendations.map(({ dish, reason }) => <div key={dish.id} className="mt-4 border-t border-border pt-4">
        <button className="min-h-11 text-left font-semibold" onClick={() => navigate(`/dishes/${dish.id}`)}>{dish.name} →</button>
        <p className="mb-3 text-xs leading-5 text-text2">{reason}</p>
        <button disabled={busy || !date} className={button} onClick={() => void act(() => healthApi.plan(dish.id, date, meal, report.period_days), "已加入个人菜单与买菜清单")}>安排这道菜</button>
      </div>)}
    </section>
    {report.plans.length > 0 && <section className={section}><h2 className="font-bold">已安排与复盘</h2>{report.plans.map(p => <div key={p.id} className="mt-4 border-t border-border pt-4">
      <p className="text-sm">{p.meal_date} · {mealNames[p.meal_type]} · {p.dish_name}</p>
      <p className="my-2 text-xs text-text2">{p.status === "recorded" ? "已有菜谱用餐记录支持完成" : p.status === "unconfirmed" ? "日期已过，尚无实际记录" : "已计划，尚未记为吃过"}{!p.available ? " · 当前菜谱不可用或与忌口冲突，请重新安排" : ""}</p>
      <button disabled={busy} className={button} onClick={() => void act(() => healthApi.cancelPlan(p.id), "已撤销计划，实际用餐记录保留")}>撤销安排</button>
    </div>)}<button className={`${button} mt-4`} onClick={() => navigate("/plan")}>查看菜单与买菜清单</button></section>}
    <details className={section}><summary className="cursor-pointer py-2 text-sm font-semibold">统计口径与数据说明</summary>{report.method_notes.map(text => <p key={text} className="mt-3 text-xs leading-6 text-text2">{text}</p>)}<p className="mt-3 text-xs text-text2">规则版本：{report.rule_version}</p></details>
  </div>
}
