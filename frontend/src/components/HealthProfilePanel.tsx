import { useEffect, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { ApiError, errorMessage, healthApi } from "@/api"
import { getSessionEpoch, isCurrentSession } from "@/api/client"
import RequestState from "@/components/RequestState"
import type { HealthProfile, HealthProfileInput } from "@/types"

const goals = [{ value: "", label: "暂不选择" }, { value: "balanced", label: "均衡饮食" }, { value: "weight_management", label: "体重管理" }, { value: "regular_meals", label: "规律吃饭" }]
const patterns = [{ value: "", label: "暂不选择" }, { value: "home", label: "以在家做饭为主" }, { value: "eating_out", label: "以外食为主" }, { value: "mixed", label: "在家与外食都有" }]
const terms = (value: string) => value.split(/[,，、;；\n]/).map(s => s.trim()).filter(Boolean)

export default function HealthProfilePanel() {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [profile, setProfile] = useState<HealthProfile | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState("")
  const [failure, setFailure] = useState("")
  const [conflict, setConflict] = useState(false)
  const request = useRef(0)
  const profileEpoch = useRef<number | null>(null)
  useEffect(() => () => { request.current++ }, [])

  async function load() {
    const id = ++request.current, epoch = getSessionEpoch()
    setOpen(true); setLoading(true); setLoadError(null); setFailure(""); setConflict(false); setMessage("")
    try {
      const next = await healthApi.profile()
      if (id === request.current && isCurrentSession(epoch)) { profileEpoch.current = epoch; setProfile(next) }
    } catch (err) {
      if (id === request.current && isCurrentSession(epoch)) setLoadError(err)
    } finally {
      if (id === request.current && isCurrentSession(epoch)) setLoading(false)
    }
  }

  async function save(input: HealthProfileInput | null) {
    if (busy || !profile) return
    if (profileEpoch.current === null || !isCurrentSession(profileEpoch.current)) {
      request.current++; setProfile(null); setOpen(false); return
    }
    const id = ++request.current, epoch = getSessionEpoch()
    setBusy(true); setFailure(""); setMessage(""); setConflict(false)
    try {
      const next = input ? await healthApi.saveProfile(input) : await healthApi.clearProfile(profile.version)
      if (id !== request.current || !isCurrentSession(epoch)) return
      setProfile(next)
      setMessage(input ? "已保存，下一次推荐会使用当前忌口。" : "已清除档案及历史，原有饮食偏好保持不变。")
      await Promise.all(["health-report", "week-plan", "pick"].map(key => qc.invalidateQueries({ queryKey: [key] })))
    } catch (err) {
      if (id !== request.current || !isCurrentSession(epoch)) return
      setFailure(errorMessage(err))
      setConflict(err instanceof ApiError && err.status === 409)
    } finally {
      if (id === request.current && isCurrentSession(epoch)) setBusy(false)
    }
  }

  return <section className="mb-5 rounded-[28px] border border-border bg-card p-5" aria-label="我的饮食档案">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div><h2 className="text-base font-bold">我的饮食档案</h2><p className="mt-1 text-xs text-text2">自愿填写，跳过也能记录和看报告</p></div>
      <button type="button" className="min-h-11 text-sm font-semibold text-primary" disabled={busy} aria-expanded={open} onClick={() => { if (open) { request.current++; setOpen(false) } else void load() }}>{open ? "收起" : "查看与填写"}</button>
    </div>
    {open && <div className="mt-4 border-t border-border pt-4">
      <p className="mb-4 text-xs leading-relaxed text-text2">仅本人可查看，不与家庭或 AI 共享。过敏与禁忌用于个人推荐和报告计划的食材筛选，仍需核对配料；家庭菜单需单独确认。目标用于记录你的意愿，暂不计算每日摄入目标。</p>
      {loading || loadError ? <RequestState error={loadError} loading={loading} onRetry={() => void load()} compact /> : profile && <HealthProfileForm key={profile.version} profile={profile} busy={busy} onSave={save} />}
      {failure && <p className="mt-3 text-sm text-text" role="alert">{failure}</p>}
      {conflict && <button type="button" className="mt-2 min-h-11 text-sm font-semibold text-primary underline" onClick={() => void load()}>放弃本次编辑，载入最新档案</button>}
      {message && <p className="mt-3 text-sm text-text2" role="status">{message}</p>}
    </div>}
  </section>
}

function HealthProfileForm({ profile, busy, onSave }: { profile: HealthProfile; busy: boolean; onSave: (input: HealthProfileInput | null) => Promise<void> }) {
  const [goal, setGoal] = useState(profile.goal)
  const [pattern, setPattern] = useState(profile.eating_pattern)
  const [allergies, setAllergies] = useState(profile.allergies.join("、"))
  const [exclusions, setExclusions] = useState(profile.dietary_exclusions.join("、"))
  const [confirmed, setConfirmed] = useState(false)
  const [erase, setErase] = useState(false)
  const field = "mt-1 min-h-11 w-full rounded-xl border border-border bg-bg px-3 py-2 text-sm text-text focus-visible:outline-2 focus-visible:outline-primary"
  return <form onSubmit={event => { event.preventDefault(); if (confirmed) void onSave({ version: profile.version, goal, eating_pattern: pattern, allergies: terms(allergies), dietary_exclusions: terms(exclusions), confirmed: true }) }}>
    <fieldset disabled={busy} className="space-y-4" onChange={() => setErase(false)}>
      <label className="block text-xs font-semibold text-text2">我想关注<select className={field} value={goal} onChange={e => { setGoal(e.target.value); setConfirmed(false) }}>{goals.map(x => <option key={x.value} value={x.value}>{x.label}</option>)}</select></label>
      <label className="block text-xs font-semibold text-text2">平常怎么吃<select className={field} value={pattern} onChange={e => { setPattern(e.target.value); setConfirmed(false) }}>{patterns.map(x => <option key={x.value} value={x.value}>{x.label}</option>)}</select></label>
      <label className="block text-xs font-semibold text-text2">过敏食材<textarea rows={2} maxLength={820} className={field} value={allergies} onChange={e => { setAllergies(e.target.value); setConfirmed(false) }} placeholder="例如花生、虾；不清楚可留空" /></label>
      <label className="block text-xs font-semibold text-text2">生活方式或宗教禁忌食材<textarea rows={2} maxLength={820} className={field} value={exclusions} onChange={e => { setExclusions(e.target.value); setConfirmed(false) }} placeholder="填写具体食材，例如猪肉、牛肉" /></label>
      <p className="text-xs leading-relaxed text-text2">每类最多 20 项，每项最多 40 字，用逗号或顿号分隔。仅仅不喜欢的口味请在「我的 → 饮食偏好」设置；已有偏好仍共同生效。留空表示未提供，不代表已确认没有过敏。</p>
      <label className="flex min-h-11 items-start gap-2 text-sm leading-relaxed"><input type="checkbox" className="mt-1.5 size-4 shrink-0 accent-primary" checked={confirmed} onChange={e => setConfirmed(e.target.checked)} />我已核对，愿意保存并用于个人推荐筛选</label>
      <button className="btn-primary min-h-11 w-full disabled:opacity-50" disabled={busy || !confirmed}>{busy ? "保存中…" : "确认保存档案"}</button>
      {profile.active && <>{profile.confirmed_at && <p className="text-xs text-text2">最近确认：{new Date(profile.confirmed_at).toLocaleDateString("zh-CN", { timeZone: "Asia/Shanghai" })}</p>}
        {erase ? <div className="rounded-xl border border-border p-3"><p className="text-sm">清除本档案及所有历史版本？保存的过敏与禁忌将不再参与筛选；饮食记录和原有偏好保留。</p><div className="mt-2 flex flex-wrap gap-4"><button type="button" className="min-h-11 text-sm font-semibold text-primary" onClick={() => void onSave(null)}>确认清除档案及历史</button><button type="button" className="min-h-11 text-sm" onClick={() => setErase(false)}>保留档案</button></div></div> : <button type="button" className="min-h-11 text-xs text-text2 underline" onClick={() => setErase(true)}>清除档案及历史</button>}
      </>}
    </fieldset>
  </form>
}
