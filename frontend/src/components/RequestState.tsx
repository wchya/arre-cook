import { AlertCircle, RefreshCw } from "lucide-react"
import { errorMessage } from "@/api/client"

export default function RequestState({ error, onRetry, loading = false, compact = false }: {
  error?: unknown
  onRetry?: () => void
  loading?: boolean
  compact?: boolean
}) {
  return (
    <div className={`request-state ${compact ? "request-state--compact" : ""}`} role={error ? "alert" : "status"} aria-live="polite">
      {error ? <AlertCircle size={28} className="text-primary" /> : <div className="skeleton h-10 w-10 rounded-2xl" />}
      <div className="font-semibold text-text">{error ? "暂时没能加载内容" : "正在准备内容…"}</div>
      {error ? <p className="max-w-sm text-sm text-text2">{errorMessage(error, "请检查网络后重试，你已保存的内容不会丢失。")}</p> : <div className="skeleton h-3 w-36 rounded-full" />}
      {Boolean(error) && onRetry && <button type="button" className="btn-secondary" onClick={onRetry} disabled={loading}><RefreshCw size={16} className={loading ? "animate-spin" : ""} />{loading ? "重试中…" : "重新加载"}</button>}
    </div>
  )
}
