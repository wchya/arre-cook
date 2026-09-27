import { useEffect, useRef, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { LoaderCircle, WandSparkles } from "lucide-react"
import { linkPreviewApi } from "@/api"
import { ApiError, errorMessage } from "@/api/client"
import { extractVideoRecipe, videoRecipeStatus, type VideoRecipeResult, type VideoRecipeStatus } from "@/api/video-recipe"
import type { AssistantQuota, DishVideoMeta } from "@/types"

interface Props {
  url: string
  userId: number
  disabled: boolean
  onStart: () => (result: VideoRecipeResult) => number
  onBusy: (busy: boolean) => void
}

export default function VideoRecipeImport({ url, userId, disabled, onStart, onBusy }: Props) {
  const queryClient = useQueryClient()
  const [busy, setBusy] = useState(false)
  const status = useQuery({ queryKey: ["video-recipe-status", userId], queryFn: videoRecipeStatus, enabled: !busy, staleTime: 15000, retry: false })
  const setQuota = (quota: AssistantQuota) => queryClient.setQueryData<VideoRecipeStatus>(["video-recipe-status", userId], (previous) => previous ? { ...previous, quota } : previous)
  const [manual, setManual] = useState(false)
  const [transcript, setTranscript] = useState("")
  const [progress, setProgress] = useState("")
  const [error, setError] = useState("")
  const [report, setReport] = useState<{ input: string; result: VideoRecipeResult; count: number } | null>(null)
  const [preview, setPreview] = useState<{ input: string; meta: DishVideoMeta } | null>(null)
  const request = useRef<AbortController | null>(null)
  const mounted = useRef(true)
  const actualQuota = status.data?.quota
  const exhausted = actualQuota && (actualQuota.remaining <= 0 || Boolean(actualQuota.blocked_reason))
  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false; request.current?.abort(); request.current = null }
  }, [])
  useEffect(() => {
    let active = true
    const timer = window.setTimeout(() => {
      if (!/https?:\/\//i.test(url)) return
      void linkPreviewApi.get(url).then((meta) => { if (active) setPreview({ input: url, meta }) }).catch(() => undefined)
    }, 700)
    return () => { active = false; window.clearTimeout(timer) }
  }, [url])
  useEffect(() => {
    return () => { request.current?.abort(); request.current = null }
  }, [url, userId, manual, transcript])

  function cancel() {
    request.current?.abort()
    request.current = null
    setBusy(false); onBusy(false); setProgress("已取消，已填写的内容会保留")
  }
  async function start() {
    if (request.current || disabled || !url.trim() || !status.data?.enabled || exhausted) return
    if (manual && (transcript.trim().length < 20 || transcript.length > 8000)) { setError("请粘贴 20–8000 字的字幕或视频文稿"); return }
    const controller = new AbortController()
    request.current = controller
    void queryClient.cancelQueries({ queryKey: ["video-recipe-status", userId], exact: true })
    const apply = onStart()
    setBusy(true); onBusy(true); setError(""); setReport(null); setProgress("正在读取视频内容…")
    try {
      const result = await extractVideoRecipe({ url: url.trim(), ...(manual ? { transcript: transcript.trim() } : {}) }, (event) => {
        if (request.current !== controller) return
        if (event.event === "status") setProgress(event.data.message)
        if (event.event === "quota") setQuota(event.data)
      }, controller.signal)
      if (request.current !== controller || controller.signal.aborted) return
      request.current = null
      const count = apply(result)
      setReport({ input: url, result, count }); setProgress("")
    } catch (err) {
      if (!controller.signal.aborted && request.current === controller) {
        setError(errorMessage(err, "提炼未完成，请稍后重试，已填写的内容会保留"))
        if (err instanceof ApiError && err.data && typeof err.data === "object" && "quota" in err.data) setQuota((err.data as { quota: AssistantQuota }).quota)
      }
    } finally {
      // Cleanup after an input change must not reset a newer request's state.
      if (mounted.current && (request.current === controller || request.current === null)) {
        request.current = null; setBusy(false); onBusy(false)
      }
      if (mounted.current) void status.refetch()
    }
  }
  const shown = report && (report.input === url || report.result.source.url === url) ? report : null
  const meta = preview?.input === url ? preview.meta : null
  return <div className="space-y-3 rounded-xl border border-border bg-bg p-3.5">
    {meta && <div className="flex items-start gap-3">
      {meta.cover && <img src={meta.cover} alt="" referrerPolicy="no-referrer" className="h-16 w-20 shrink-0 rounded-lg object-cover" />}
      <div className="min-w-0 text-xs leading-relaxed"><div className="font-semibold text-primary">{meta.platform_name}{meta.duration ? ` · ${meta.duration}` : ""}</div><p className="break-words text-text">{meta.title}</p>{meta.author && <p className="text-text2">{meta.author}</p>}</div>
    </div>}
    <div><div className="text-sm font-semibold">从视频提炼做法</div><p className="mt-1 text-xs leading-relaxed text-text2">支持 10 分钟以内的公开视频。{status.data?.asr_enabled ? "字幕或视频将交给 AI 服务提炼。" : "优先读取公开字幕并交给 AI 提炼；无法读取时，可粘贴字幕或文稿。"}仅填入空白项，核对后再保存。</p></div>
    <button type="button" className="min-h-11 text-xs font-semibold text-primary underline underline-offset-4" disabled={busy} onClick={() => { setManual(!manual); setError("") }}>{manual ? "返回视频自动读取" : "粘贴字幕或视频文稿"}</button>
    {manual && <label className="block text-xs font-semibold">字幕 / 视频文稿<textarea aria-label="字幕 / 视频文稿" value={transcript} maxLength={8000} disabled={busy} onChange={(event) => setTranscript(event.target.value)} placeholder="粘贴这道菜的完整做法，不必整理格式（20–8000 字）" className="mt-2 min-h-32 w-full rounded-xl border border-border bg-card p-3 text-sm font-normal leading-relaxed outline-none focus:border-primary" /><span className="text-text2">{transcript.length} / 8000 字 · 文稿将用于 AI 提炼</span></label>}
    {status.isError && <p className="text-xs text-text2">提炼服务状态加载失败。<button type="button" className="min-h-11 text-primary underline" onClick={() => { void status.refetch() }}>重新加载</button></p>}
    {status.data && !status.data.enabled && <p className="text-xs text-text2">管理员尚未启用 AI 提炼，可以继续手动填写菜谱。</p>}
    {actualQuota && <p className="text-xs text-text2">{exhausted ? "今日 AI 次数已用完或服务已暂停，仍可手动填写。" : `今日剩余 ${actualQuota.remaining} 次，与 AI 助手共用。开始 AI 处理后计 1 次。`}</p>}
    <div className="flex flex-wrap gap-2">
      <button type="button" disabled={disabled || busy || !url.trim() || !status.data?.enabled || Boolean(exhausted)} onClick={() => { void start() }} className="inline-flex min-h-11 items-center justify-center gap-2 rounded-xl bg-primary px-4 text-sm font-semibold text-white disabled:opacity-50">{busy ? <LoaderCircle size={16} className="animate-spin" /> : <WandSparkles size={16} />}{busy ? "提炼中…" : manual ? "从字幕提炼" : "从视频提炼"}</button>
      {busy && <button type="button" className="btn-secondary min-h-11" onClick={cancel}>取消</button>}
    </div>
    {busy && <p role="status" aria-live="polite" className="text-xs text-text2">{progress}</p>}
    {error && <p role="alert" className="text-xs leading-relaxed text-red-600 dark:text-red-400">{error}</p>}
    {shown && <div role="status" className="space-y-2 text-xs leading-relaxed text-text2">
      <p className="font-semibold text-primary">{shown.count ? `已填入 ${shown.count} 项空白内容，核对后保存。` : "已提炼完成。当前字段已有内容，可展开结果核对。"}</p>
      <p>依据：{({ subtitle: "视频字幕", audio: "视频语音转写", manual: "你粘贴的字幕 / 文稿" })[shown.result.source.method]}。视频未注明的信息请自行补充。</p>
      <details><summary className="min-h-11 cursor-pointer py-2 font-semibold text-text">查看提炼结果与字幕依据</summary><p className="font-semibold">{shown.result.recipe.name}</p><p>食材：{[...shown.result.recipe.ingredients, ...shown.result.recipe.seasonings].map((item) => `${item.name} ${item.amount || "用量未注明"}`).join("、")}</p><ol className="mt-2 list-decimal space-y-2 pl-4">{shown.result.recipe.steps.map((step, index) => <li key={index}><p>{step.text}</p><p className="mt-1 text-text2">原句：{step.evidence}</p></li>)}</ol></details>
    </div>}
  </div>
}
