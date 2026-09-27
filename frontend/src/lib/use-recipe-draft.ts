import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react"
import { readRecipeDraft, recipeDraftKey, type RecipeDraftFields } from "./recipe-draft"

export function useRecipeDraft(userId: number, recipeId: number, mode: string, value: RecipeDraftFields, restore: (value: RecipeDraftFields) => void) {
  const key = recipeDraftKey(userId, recipeId, mode)
  const [recovery, setRecovery] = useState(() => readRecipeDraft(key))
  const [status, setStatus] = useState("草稿会自动保存在本设备")
  const serialized = JSON.stringify(value)
  const baseline = useRef(serialized)
  const latest = useRef(serialized)
  const blocked = useRef(Boolean(recovery))
  const committed = useRef(false)
  const lastSaved = useRef("")

  useLayoutEffect(() => { latest.current = serialized; blocked.current = Boolean(recovery) }, [serialized, recovery])

  const persist = useCallback((showStatus: boolean) => {
      if (!userId || committed.current || blocked.current || latest.current === lastSaved.current) return
      try {
        if (latest.current === baseline.current) {
          if (lastSaved.current) localStorage.removeItem(key)
          lastSaved.current = latest.current
          return
        }
        localStorage.setItem(key, JSON.stringify({ version: 1, savedAt: Date.now(), value: JSON.parse(latest.current) }))
        lastSaved.current = latest.current
        if (showStatus) setStatus("草稿已保存 · 仅本设备、当前账号可恢复")
      } catch { if (showStatus) setStatus("草稿未能保存，请完成保存后再离开") }
  }, [key, userId])
  useEffect(() => {
    const timer = window.setTimeout(() => persist(true), 450)
    return () => clearTimeout(timer)
  }, [persist, serialized, recovery])
  useEffect(() => {
    const flush = () => persist(false)
    window.addEventListener("pagehide", flush)
    return () => {
      window.removeEventListener("pagehide", flush)
      persist(false)
    }
  }, [persist])

  function clear() {
    committed.current = true
    try { localStorage.removeItem(key) } catch { /* The saved recipe is already on the server. */ }
  }

  function resume() {
    if (!recovery) return
    restore(recovery.value)
    setRecovery(null)
    setStatus("已恢复草稿，确认后保存菜谱")
  }

  function discard() {
    try {
      localStorage.removeItem(key)
      setRecovery(null)
      setStatus("已丢弃旧草稿，新的修改会自动保存")
    } catch { setStatus("暂时无法清除草稿，请检查浏览器存储空间") }
  }

  return { recovery, status, clear, resume, discard }
}
