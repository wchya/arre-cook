import { useEffect, useMemo, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { errorMessage, meApi, settingsApi } from "@/api"
import { asArray } from "@/lib/utils"
import PageHeader from "@/components/PageHeader"
import TagInput from "@/components/TagInput"
import type { Preferences as Prefs } from "@/types"
import toast from "react-hot-toast"
import { Salad } from "lucide-react"

const SPICE = [
  { v: -1, label: "不限", emoji: "🙂" },
  { v: 0, label: "不吃辣", emoji: "🥛" },
  { v: 1, label: "微辣", emoji: "🌶" },
  { v: 2, label: "中辣", emoji: "🌶🌶" },
  { v: 3, label: "特辣", emoji: "🔥" },
]
const COOK_TIMES = [0, 20, 30, 45, 60]
const GOALS = ["减脂", "增肌", "控糖", "清淡养胃", "快手省事", "宝宝辅食"]
const COMMON_AVOID = ["香菜", "葱", "蒜", "内脏", "羊肉", "鱼", "肥肉", "芹菜", "苦瓜"]
const COMMON_ALLERGY = ["花生", "虾", "蟹", "贝类", "鸡蛋", "牛奶", "大豆", "芝麻", "坚果"]
const DEFAULT_TASTES = ["麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "酱香"]

const empty: Prefs = { avoid_ingredients: [], allergies: [], favorite_tastes: [], spice_level: -1, household_size: 0, max_cook_time: 0, goals: "", notes: "" }

function Section({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <section className="rounded-[22px] border border-border bg-card p-4 shadow-[0_1px_3px_rgba(0,0,0,.03),0_8px_24px_rgba(0,0,0,.04)]">
      <div className="mb-0.5 text-[15px] font-extrabold">{title}</div>
      {hint && <div className="mb-3 text-[11px] leading-relaxed text-text3">{hint}</div>}
      {children}
    </section>
  )
}

export default function Preferences() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ["preferences"], queryFn: () => meApi.preferences() })
  const { data: settings } = useQuery({ queryKey: ["settings"], queryFn: () => settingsApi.get() })
  const tasteOptions = useMemo(() => {
    const list = asArray<string>(settings?.tastes).filter((x) => typeof x === "string")
    return list.length > 0 ? list : DEFAULT_TASTES
  }, [settings])

  const [form, setForm] = useState<Prefs>(empty)
  const [dirty, setDirty] = useState(false)
  useEffect(() => {
    if (data) {
      // Hydrate the editable form after the server preference query resolves.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setForm({ ...empty, ...data })
      setDirty(false)
    }
  }, [data])

  const patch = (p: Partial<Prefs>) => {
    setForm((f) => ({ ...f, ...p }))
    setDirty(true)
  }

  const saveMut = useMutation({
    mutationFn: () => meApi.updatePreferences(form),
    onSuccess: (res) => {
      qc.setQueryData(["preferences"], res)
      qc.invalidateQueries({ queryKey: ["profile"] })
      qc.invalidateQueries({ queryKey: ["week-plan"] })
      qc.invalidateQueries({ queryKey: ["pick"] })
      setDirty(false)
      toast.success("已保存，推荐会按新偏好来")
    },
    onError: (err) => toast.error(errorMessage(err, "保存失败")),
  })

  const goalList = form.goals ? form.goals.split(/[,，、\s]+/).filter(Boolean) : []

  return (
    <div className="animate-fadeUp pb-28">
      <PageHeader title="饮食偏好" subtitle="推荐引擎和 AI 助手都会遵守" icon={Salad} onBack={() => navigate(-1)} />
      {isLoading ? (
        <div className="mx-auto max-w-[640px] space-y-3 px-5 py-4">
          {[1, 2, 3].map((i) => <div key={i} className="skeleton h-32 rounded-[22px]" />)}
        </div>
      ) : (
        <div className="mx-auto max-w-[640px] space-y-3 px-5 py-4">
          <Section title="🚫 过敏原" hint="含有这些食材的菜一律不推荐（任何情况下都不会放宽）">
            <TagInput value={form.allergies} onChange={(v) => patch({ allergies: v })} placeholder="如：花生、虾" suggestions={COMMON_ALLERGY} tone="red" />
          </Section>

          <Section title="🙅 忌口" hint="尽量避开；实在没得选时才会放宽">
            <TagInput value={form.avoid_ingredients} onChange={(v) => patch({ avoid_ingredients: v })} placeholder="如：香菜、内脏" suggestions={COMMON_AVOID} />
          </Section>

          <Section title="🌶 能吃多辣">
            <div className="grid grid-cols-5 gap-1.5">
              {SPICE.map((s) => (
                <button
                  key={s.v}
                  onClick={() => patch({ spice_level: s.v })}
                  className={`flex flex-col items-center gap-0.5 rounded-2xl border-[1.5px] py-2.5 text-[11px] font-bold transition-all active:scale-95 ${form.spice_level === s.v ? "border-primary bg-primary-light text-primary" : "border-border bg-bg text-text2"}`}
                >
                  <span className="text-[15px] leading-none">{s.emoji}</span>
                  {s.label}
                </button>
              ))}
            </div>
          </Section>

          <Section title="😋 喜欢的口味" hint="会作为口味画像的起点，新用户也能一上来就推得准">
            <div className="flex flex-wrap gap-1.5">
              {tasteOptions.map((t) => {
                const on = form.favorite_tastes.includes(t)
                return (
                  <button
                    key={t}
                    onClick={() => patch({ favorite_tastes: on ? form.favorite_tastes.filter((x) => x !== t) : [...form.favorite_tastes, t] })}
                    className={`rounded-full border px-3 py-1.5 text-[12px] font-semibold transition-all active:scale-95 ${on ? "border-pink bg-pink text-white" : "border-border bg-bg text-text2"}`}
                  >
                    {t}
                  </button>
                )
              })}
            </div>
          </Section>

          <Section title="⏱ 一道菜最多花多久">
            <div className="grid grid-cols-5 gap-1.5">
              {COOK_TIMES.map((m) => (
                <button
                  key={m}
                  onClick={() => patch({ max_cook_time: m })}
                  className={`rounded-2xl border-[1.5px] py-2.5 text-[12px] font-bold transition-all active:scale-95 ${form.max_cook_time === m ? "border-mint bg-mint-light text-mint" : "border-border bg-bg text-text2"}`}
                >
                  {m === 0 ? "不限" : `${m} 分钟`}
                </button>
              ))}
            </div>
          </Section>

          <Section title="👨‍👩‍👧 几个人吃饭">
            <div className="flex items-center gap-3">
              <button onClick={() => patch({ household_size: Math.max(0, form.household_size - 1) })} className="h-10 w-10 rounded-full border border-border bg-bg text-lg font-bold active:scale-95">−</button>
              <div className="min-w-[72px] text-center text-[18px] font-black">{form.household_size === 0 ? "未设置" : `${form.household_size} 人`}</div>
              <button onClick={() => patch({ household_size: Math.min(20, form.household_size + 1) })} className="h-10 w-10 rounded-full border border-border bg-bg text-lg font-bold active:scale-95">+</button>
            </div>
          </Section>

          <Section title="🎯 饮食目标">
            <div className="flex flex-wrap gap-1.5">
              {GOALS.map((g) => {
                const on = goalList.includes(g)
                return (
                  <button
                    key={g}
                    onClick={() => patch({ goals: (on ? goalList.filter((x) => x !== g) : [...goalList, g]).join("、") })}
                    className={`rounded-full border px-3 py-1.5 text-[12px] font-semibold transition-all active:scale-95 ${on ? "border-purple bg-purple-light text-purple" : "border-border bg-bg text-text2"}`}
                  >
                    {g}
                  </button>
                )
              })}
            </div>
          </Section>

          <Section title="📝 给 AI 的补充说明" hint="例如：孩子不吃辣、周末喜欢做硬菜、最近在控油">
            <textarea
              value={form.notes}
              maxLength={500}
              onChange={(e) => patch({ notes: e.target.value })}
              rows={3}
              className="w-full resize-none rounded-2xl border-[1.5px] border-border bg-bg px-4 py-3 text-[14px] leading-relaxed outline-none focus:border-primary focus:bg-card"
            />
          </Section>
        </div>
      )}

      <div className="fixed inset-x-0 bottom-0 z-[120] border-t border-border bg-bg/92 px-5 pt-3 backdrop-blur-xl" style={{ paddingBottom: "calc(12px + env(safe-area-inset-bottom))" }}>
        <button
          onClick={() => saveMut.mutate()}
          disabled={!dirty || saveMut.isPending}
          className="mx-auto block h-12 w-full max-w-[640px] rounded-2xl bg-primary text-[15px] font-extrabold text-white shadow-[0_10px_24px_rgba(232,115,74,.3)] transition-all active:scale-[.98] disabled:opacity-40 disabled:shadow-none"
        >
          {saveMut.isPending ? "保存中…" : dirty ? "保存偏好" : "已保存"}
        </button>
      </div>
    </div>
  )
}
