import { api, ApiError, getToken } from "./client"
import type { AssistantQuota, DishIngredient, DishStep } from "@/types"

export interface VideoRecipe {
  name: string
  ingredients: Array<DishIngredient & { evidence: string }>
  seasonings: Array<DishIngredient & { evidence: string }>
  steps: Array<DishStep & { evidence: string }>
  cook_time: number
  remark: string
}
export interface VideoRecipeResult {
  recipe: VideoRecipe
  source: { url: string; platform: string; method: "subtitle" | "audio" | "manual"; text: string }
}
export interface VideoRecipeStatus {
  enabled: boolean
  asr_enabled: boolean
  quota: AssistantQuota
}
export const videoRecipeStatus = () => api<VideoRecipeStatus>("GET", "/assistant/video-recipe/status")

export type VideoProgress = { event: "status"; data: { message: string } } | { event: "quota"; data: AssistantQuota }

// Only return a complete, approved result followed by done. A broken connection
// must never silently apply a partial draft. No automatic POST retries.
export async function extractVideoRecipe(body: { url: string; transcript?: string }, onProgress: (event: VideoProgress) => void, signal: AbortSignal): Promise<VideoRecipeResult> {
  const token = getToken()
  const controller = new AbortController()
  const abort = () => controller.abort()
  signal.addEventListener("abort", abort, { once: true })
  if (signal.aborted) abort()
  const timeout = window.setTimeout(abort, 115000)
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined
  try {
    const res = await fetch("/api/assistant/video-recipe", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "text/event-stream", Authorization: `Bearer ${token}` },
      body: JSON.stringify(body), signal: controller.signal,
    })
    if (token !== getToken()) throw new Error("登录状态已变化，请重新提炼")
    if (res.status === 401) {
      window.dispatchEvent(new Event("auth-expired"))
      throw new ApiError("登录已过期", 40100, 401)
    }
    if (!res.ok) {
      const error = await res.json().catch(() => null)
      throw new ApiError(error?.message || "提炼暂时不可用，请稍后再试", error?.code || -1, res.status, error?.data)
    }
    if (!res.body || !res.headers.get("content-type")?.includes("text/event-stream")) throw new Error("未收到提炼结果，请稍后重试")
    reader = res.body.getReader()
    const decoder = new TextDecoder("utf-8", { fatal: true })
    let buffer = "", total = 0, doneEvent = false
    let result: VideoRecipeResult | undefined
    for (;;) {
      const chunk = await reader.read()
      if (token !== getToken()) throw new Error("登录状态已变化，请重新提炼")
      if (chunk.done) break
      total += chunk.value.byteLength
      if (total > 128 * 1024) throw new Error("提炼结果过长，请缩短字幕后重试")
      buffer += decoder.decode(chunk.value, { stream: true })
      let match: RegExpMatchArray | null
      while ((match = buffer.match(/\r?\n\r?\n/))) {
        const frame = buffer.slice(0, match.index)
        buffer = buffer.slice((match.index || 0) + match[0].length)
        let event = ""
        const lines: string[] = []
        frame.split(/\r?\n/).forEach((line) => {
          if (line.startsWith("event:")) event = line.slice(6).trim()
          if (line.startsWith("data:")) lines.push(line.slice(5).trimStart())
        })
        if (!lines.length) continue
        const data = JSON.parse(lines.join("\n"))
        if (event === "error") throw new ApiError(data.message || "提炼失败，请稍后重试", -1, 200, data)
        if (event === "status" || event === "quota") onProgress({ event, data } as VideoProgress)
        if (event === "recipe") {
          if (result || doneEvent || !validResult(data)) throw new Error("提炼结果格式不完整，请重试")
          result = data
        }
        if (event === "done") doneEvent = true
      }
    }
    decoder.decode()
    if (signal.aborted || controller.signal.aborted) throw new DOMException("Aborted", "AbortError")
    if (!doneEvent || !result || buffer.trim()) throw new Error("连接中断，草稿未被改动，请稍后重试")
    return result
  } finally {
    window.clearTimeout(timeout)
    signal.removeEventListener("abort", abort)
    await reader?.cancel().catch(() => undefined)
    reader?.releaseLock()
    controller.abort()
  }
}

function validResult(value: unknown): value is VideoRecipeResult {
  if (!value || typeof value !== "object") return false
  const v = value as VideoRecipeResult
  const r = v.recipe
  return Boolean(r && typeof r.name === "string" && typeof r.remark === "string" && Number.isInteger(r.cook_time) && r.cook_time >= 0 && r.cook_time <= 600 &&
    [r.ingredients, r.seasonings].every((items) => Array.isArray(items) && items.length <= 25 && items.every((item) => item && typeof item.name === "string" && typeof item.amount === "string" && typeof item.evidence === "string")) &&
    Array.isArray(r.steps) && r.steps.length > 0 && r.steps.length <= 30 && r.steps.every((step) => step && typeof step.text === "string" && Number.isInteger(step.time) && typeof step.evidence === "string") &&
    v.source && ["subtitle", "audio", "manual"].includes(v.source.method) && typeof v.source.url === "string" && typeof v.source.text === "string")
}
