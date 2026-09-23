import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { achievementsApi } from "@/api"
import type { Achievement } from "@/types"
import toast from "react-hot-toast"

const emojiOptions = ["🍳", "🥢", "🔥", "💯", "🥘", "🍲", "⚡", "👨‍🍳", "🌶️", "🗺️", "🌙", "🥗", "🏠", "👅", "🍽️", "🌸", "🌅", "📸", "❤", "🛒", "🎯", "🎁", "📅", "📚", "📝", "⭐", "🏆", "💎", "🌟", "🧺"]

export default function AdminAchievements() {
  const qc = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [editId, setEditId] = useState<number | null>(null)
  const [form, setForm] = useState({ code: "", name: "", description: "", icon: "🏆", condition: "manual" })

  const { data: rawAchievements, isLoading } = useQuery({
    queryKey: ["achievements"],
    queryFn: () => achievementsApi.list(),
  })
  const achievements = Array.isArray(rawAchievements) ? rawAchievements : []

  const createMut = useMutation({
    mutationFn: (data: Partial<Achievement>) => achievementsApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success("成就已创建")
      resetForm()
    },
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Achievement> }) => achievementsApi.update(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success("成就已更新")
      resetForm()
    },
  })

  const deleteMut = useMutation({
    mutationFn: (id: number) => achievementsApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success("成就已删除")
    },
  })

  function resetForm() {
    setForm({ code: "", name: "", description: "", icon: "🏆", condition: "manual" })
    setEditId(null)
    setShowForm(false)
  }

  function startEdit(a: Achievement) {
    setForm({ code: a.code, name: a.name, description: a.description, icon: a.icon, condition: a.condition })
    setEditId(a.id)
    setShowForm(true)
  }

  function handleSubmit() {
    if (!form.code || !form.name || !form.description) {
      toast.error("请填写必填项")
      return
    }
    if (editId) {
      updateMut.mutate({ id: editId, data: form })
    } else {
      createMut.mutate(form)
    }
  }

  if (isLoading) return <div className="p-8 text-center text-text2">加载中...</div>

  return (
    <div className="px-5 py-4 max-w-[640px] mx-auto pb-20">
      <div className="flex items-center justify-between mb-3.5">
        <div className="text-lg font-bold">成就管理</div>
        <div className="text-xs text-text3 ml-auto mr-2">{achievements.filter((a: Achievement) => a.is_unlocked).length}/{achievements.length} 已解锁</div>
        <button
          onClick={() => { resetForm(); setShowForm(!showForm) }}
          className="inline-flex items-center justify-center gap-1.5 py-1.5 px-4 rounded-full text-xs font-semibold bg-primary text-white transition-all active:scale-96"
        >
          {showForm ? "取消" : "+ 添加成就"}
        </button>
      </div>

      {showForm && (
        <div className="bg-card rounded-2xl p-4 shadow-sm border border-border mb-4">
          <div className="mb-3">
            <label className="block text-[13px] font-semibold mb-1 text-text2">图标</label>
            <div className="flex flex-wrap gap-2">
              {emojiOptions.map((e) => (
                <button
                  key={e}
                  onClick={() => setForm((f) => ({ ...f, icon: e }))}
                  className={`w-10 h-10 rounded-xl text-xl flex items-center justify-center transition-all ${form.icon === e ? "bg-primary-light border-2 border-primary" : "bg-bg border border-border"}`}
                >
                  {e}
                </button>
              ))}
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3 mb-3">
            <div>
              <label className="block text-[13px] font-semibold mb-1 text-text2">编码 *</label>
              <input
                value={form.code}
                onChange={(e) => setForm((f) => ({ ...f, code: e.target.value }))}
                className="w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
                placeholder="如 first_pick"
              />
            </div>
            <div>
              <label className="block text-[13px] font-semibold mb-1 text-text2">名称 *</label>
              <input
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                className="w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
                placeholder="如 初入厨房"
              />
            </div>
          </div>
          <div className="mb-3">
            <label className="block text-[13px] font-semibold mb-1 text-text2">描述 *</label>
            <input
              value={form.description}
              onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
              className="w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
              placeholder="如 第一次点菜"
            />
          </div>
          <div className="mb-4">
            <label className="block text-[13px] font-semibold mb-1 text-text2">条件类型</label>
            <select
              value={form.condition}
              onChange={(e) => setForm((f) => ({ ...f, condition: e.target.value }))}
              className="w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
            >
              <option value="manual">手动激活</option>
              <option value="auto">自动检测</option>
            </select>
          </div>
          <button onClick={handleSubmit} className="w-full py-2.5 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96">
            {editId ? "更新成就" : "创建成就"}
          </button>
        </div>
      )}

      <div className="bg-card rounded-2xl overflow-hidden shadow-sm border border-border">
        {achievements.map((a: Achievement) => (
          <div key={a.id} className="flex items-center gap-3 px-4 py-3 border-b border-border last:border-0">
            <div className="w-10 h-10 rounded-[10px] bg-primary-light flex items-center justify-center text-xl flex-shrink-0">
              {a.icon}
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-sm font-semibold">{a.name}</div>
              <div className="text-[11px] text-text2">{a.description} · {a.code}</div>
              <div className="flex gap-1.5 mt-1">
                <span className={`px-1.5 py-px rounded-full text-[10px] ${a.condition === "auto" ? "bg-mint-light text-mint" : "bg-bg text-text3"}`}>
                  {a.condition === "auto" ? "自动检测" : "手动"}
                </span>
                {a.is_unlocked && <span className="px-1.5 py-px rounded-full text-[10px] bg-primary-light text-primary">已解锁</span>}
              </div>
            </div>
            <div className="flex gap-1.5 flex-shrink-0">
              <button
                onClick={() => startEdit(a)}
                className="w-8 h-8 rounded-[10px] flex items-center justify-center text-sm bg-bg transition-all active:bg-primary-light"
              >
                ✎
              </button>
              <button
                onClick={() => { if (confirm(`确定删除「${a.name}」吗？`)) deleteMut.mutate(a.id) }}
                className="w-8 h-8 rounded-[10px] flex items-center justify-center text-sm bg-bg transition-all active:bg-red-light active:text-red-500"
              >
                🗑
              </button>
            </div>
          </div>
        ))}
        {achievements.length === 0 && (
          <div className="py-12 text-center text-sm text-text3">还没有成就，去添加第一个吧</div>
        )}
      </div>
    </div>
  )
}
