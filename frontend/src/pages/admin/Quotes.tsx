import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { quotesApi } from "@/api"
import type { Quote } from "@/types"
import toast from "react-hot-toast"

const scenes = [
  { key: "lunch", label: "午餐" },
  { key: "dinner", label: "晚餐" },
  { key: "quick", label: "快手菜" },
  { key: "soup", label: "汤品" },
  { key: "favorite", label: "收藏" },
  { key: "general", label: "通用" },
]

function sceneLabel(s: string) {
  return scenes.find((x) => x.key === s)?.label || s || "通用"
}

export default function AdminQuotes() {
  const qc = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [editId, setEditId] = useState<number | null>(null)
  const [form, setForm] = useState({ content: "", scene: "general" })
  const [filterScene, setFilterScene] = useState("")

  const { data: rawQuotes, isLoading } = useQuery({
    queryKey: ["quotes"],
    queryFn: () => quotesApi.list(),
  })
  const allQuotes = Array.isArray(rawQuotes) ? rawQuotes : []
  const quotes = filterScene ? allQuotes.filter((q: Quote) => q.scene === filterScene) : allQuotes

  const createMut = useMutation({
    mutationFn: (data: { content: string; scene: string }) => quotesApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["quotes"] })
      toast.success("语录已创建")
      resetForm()
    },
    onError: () => toast.error("创建失败"),
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { content: string; scene: string } }) => quotesApi.update(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["quotes"] })
      toast.success("语录已更新")
      resetForm()
    },
    onError: () => toast.error("更新失败"),
  })

  const deleteMut = useMutation({
    mutationFn: (id: number) => quotesApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["quotes"] })
      toast.success("语录已删除")
    },
  })

  function resetForm() {
    setForm({ content: "", scene: "general" })
    setEditId(null)
    setShowForm(false)
  }

  function startEdit(q: Quote) {
    setForm({ content: q.content, scene: q.scene || "general" })
    setEditId(q.id)
    setShowForm(true)
  }

  function handleSubmit() {
    if (!form.content.trim()) {
      toast.error("请填写语录内容")
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
        <div className="text-lg font-bold">语录管理</div>
        <button
          onClick={() => { resetForm(); setShowForm(!showForm) }}
          className="inline-flex items-center justify-center gap-1.5 py-1.5 px-4 rounded-full text-xs font-semibold bg-primary text-white transition-all active:scale-96"
        >
          {showForm ? "取消" : "+ 添加语录"}
        </button>
      </div>

      {showForm && (
        <div className="bg-card rounded-2xl p-4 shadow-sm border border-border mb-4">
          <div className="mb-3">
            <label className="block text-[13px] font-semibold mb-1.5 text-text2">语录内容 *</label>
            <textarea
              value={form.content}
              onChange={(e) => setForm((f) => ({ ...f, content: e.target.value }))}
              placeholder="一句温暖的话..."
              rows={3}
              className="w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none min-h-[60px] resize-y leading-relaxed transition-all focus:border-primary"
            />
          </div>
          <div className="mb-4">
            <label className="block text-[13px] font-semibold mb-1.5 text-text2">场景</label>
            <div className="flex flex-wrap gap-2">
              {scenes.map((s) => (
                <button
                  key={s.key}
                  onClick={() => setForm((f) => ({ ...f, scene: s.key }))}
                  className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${form.scene === s.key ? "bg-primary text-white border-primary" : "border-border text-text2"}`}
                >
                  {s.label}
                </button>
              ))}
            </div>
          </div>
          <button onClick={handleSubmit} className="w-full py-2.5 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96">
            {editId ? "更新语录" : "创建语录"}
          </button>
        </div>
      )}

      <div className="flex flex-wrap gap-1.5 mb-3">
        <button onClick={() => setFilterScene("")} className={`px-3 py-1.5 rounded-full text-xs border-[1.5px] transition-all whitespace-nowrap ${filterScene === "" ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>
          全部 ({allQuotes.length})
        </button>
        {scenes.map((s) => {
          const count = allQuotes.filter((q: Quote) => q.scene === s.key).length
          return (
            <button key={s.key} onClick={() => setFilterScene(s.key)} className={`px-3 py-1.5 rounded-full text-xs border-[1.5px] transition-all whitespace-nowrap ${filterScene === s.key ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>
              {s.label} ({count})
            </button>
          )
        })}
      </div>

      <div className="bg-card rounded-2xl overflow-hidden shadow-sm border border-border">
        {quotes.map((q: Quote) => (
          <div key={q.id} className="flex items-center gap-3 px-4 py-3 border-b border-border last:border-0">
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium leading-relaxed">{q.content}</div>
              <div className="flex gap-1.5 mt-1">
                <span className="px-1.5 py-px rounded-full text-[10px] bg-primary-light text-primary">{sceneLabel(q.scene)}</span>
                {!q.enabled && <span className="px-1.5 py-px rounded-full text-[10px] bg-bg text-text3">已禁用</span>}
              </div>
            </div>
            <div className="flex gap-1.5 flex-shrink-0">
              <button onClick={() => startEdit(q)} className="w-8 h-8 rounded-[10px] flex items-center justify-center text-sm bg-bg transition-all active:bg-primary-light">✎</button>
              <button onClick={() => { if (confirm("确定删除这条语录？")) deleteMut.mutate(q.id) }} className="w-8 h-8 rounded-[10px] flex items-center justify-center text-sm bg-bg transition-all active:bg-red-light active:text-red-500">🗑</button>
            </div>
          </div>
        ))}
        {quotes.length === 0 && (
          <div className="py-12 text-center text-sm text-text3">{filterScene ? "该场景下暂无语录" : "还没有语录，去添加第一条吧"}</div>
        )}
      </div>
    </div>
  )
}
