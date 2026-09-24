import { useState } from "react"
import { X } from "lucide-react"

// 标签输入：回车 / 逗号 / 空格添加，点 × 删除；可给出快捷选项
export default function TagInput({ value, onChange, placeholder, suggestions = [], tone = "primary", max = 30 }: {
  value: string[]
  onChange: (v: string[]) => void
  placeholder?: string
  suggestions?: string[]
  tone?: "primary" | "red" | "mint"
  max?: number
}) {
  const [draft, setDraft] = useState("")
  const toneCls = tone === "red" ? "bg-red-light text-red" : tone === "mint" ? "bg-mint-light text-mint" : "bg-primary-light text-primary"

  function add(raw: string) {
    const parts = raw.split(/[,，、\s]+/).map((s) => s.trim()).filter(Boolean)
    if (parts.length === 0) return
    const next = [...value]
    for (const p of parts) {
      if (!next.includes(p) && next.length < max) next.push(p)
    }
    onChange(next)
    setDraft("")
  }

  const rest = suggestions.filter((s) => !value.includes(s))

  return (
    <div>
      <div className="flex min-h-12 flex-wrap items-center gap-1.5 rounded-2xl border-[1.5px] border-border bg-bg px-2.5 py-2 transition-all focus-within:border-primary focus-within:bg-card">
        {value.map((v) => (
          <span key={v} className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[12px] font-semibold ${toneCls}`}>
            {v}
            <button onClick={() => onChange(value.filter((x) => x !== v))} aria-label={`删除${v}`} className="opacity-70 hover:opacity-100">
              <X size={12} strokeWidth={2.6} />
            </button>
          </span>
        ))}
        <input
          value={draft}
          onChange={(e) => {
            const v = e.target.value
            if (/[,，、]$/.test(v)) add(v)
            else setDraft(v)
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              add(draft)
            } else if (e.key === "Backspace" && !draft && value.length > 0) {
              onChange(value.slice(0, -1))
            }
          }}
          onBlur={() => draft && add(draft)}
          placeholder={value.length === 0 ? placeholder : "继续添加…"}
          className="min-w-[90px] flex-1 bg-transparent px-1 py-1 text-[14px] outline-none"
        />
      </div>
      {rest.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {rest.slice(0, 12).map((s) => (
            <button key={s} onClick={() => add(s)} className="rounded-full border border-dashed border-border2 px-2.5 py-1 text-[11px] font-medium text-text2 active:scale-95">
              + {s}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
