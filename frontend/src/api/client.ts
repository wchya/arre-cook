import axios from "axios"
import type { ApiResponse } from "@/types"

const client = axios.create({
  baseURL: "/api",
  timeout: 15000,
})

export const TOKEN_KEY = "ninimenu_token"

// 旧版“应用密码 / 管理密码”凭证已废弃，启动时清掉
for (const legacy of ["ninimenu_app_password", "token"]) {
  try {
    localStorage.removeItem(legacy)
  } catch {
    /* ignore */
  }
}

function storedToken(): string | null | undefined {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return undefined
  }
}

// Each tab pins its identity. Shared storage is checked before dispatch and
// before returning results, so a delayed storage event cannot mix identities.
let sessionToken = storedToken() ?? null
let sessionEpoch = 0
let memoryOnly = false
export function getToken(): string | null { return sessionToken }
export function getSessionEpoch(): number { return sessionEpoch }
export function isCurrentSession(epoch: number): boolean {
  const shared = storedToken()
  return epoch === sessionEpoch && (memoryOnly || shared === undefined || shared === sessionToken)
}
export function setToken(token: string | null) {
  if (token !== sessionToken) { sessionToken = token; sessionEpoch++ }
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token)
    else localStorage.removeItem(TOKEN_KEY)
    memoryOnly = false
  } catch { memoryOnly = true }
}
function syncSharedSession() {
  const token = storedToken()
  if (memoryOnly || token === undefined) return
  if (token === sessionToken) return
  sessionToken = token
  sessionEpoch++
  window.dispatchEvent(new window.Event("auth-storage-change"))
}
if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key === TOKEN_KEY || event.key === null) syncSharedSession()
  })
  window.addEventListener("focus", syncSharedSession)
}

declare module "axios" {
  interface InternalAxiosRequestConfig { sessionEpoch?: number }
}
export class SessionChangedError extends Error {
  constructor() { super("账号状态已变化，请在当前账号下重新操作") }
}
client.interceptors.request.use((config) => {
  if (!isCurrentSession(sessionEpoch)) {
    syncSharedSession()
    throw new SessionChangedError()
  }
  config.sessionEpoch = sessionEpoch
  if (sessionToken) config.headers.Authorization = `Bearer ${sessionToken}`
  else delete config.headers.Authorization
  return config
}, (error) => { throw error }, { synchronous: true })

client.interceptors.response.use(
  (res) => {
    if (!isCurrentSession(res.config.sessionEpoch ?? -1)) { syncSharedSession(); throw new SessionChangedError() }
    return res
  },
  (err) => {
    if (err.config && !isCurrentSession(err.config.sessionEpoch ?? -1)) { syncSharedSession(); return Promise.reject(new SessionChangedError()) }
    const url: string = err.config?.url || ""
    if (err.response?.status === 401 && !url.startsWith("/auth/") && err.config?.headers?.Authorization === `Bearer ${getToken()}`) {
      setToken(null)
      window.dispatchEvent(new window.Event("auth-expired"))
    }
    return Promise.reject(err)
  },
)

export class ApiError extends Error {
  code: number
  status: number
  data: unknown
  constructor(message: string, code: number, status: number, data?: unknown) {
    super(message)
    this.code = code
    this.status = status
    this.data = data
  }
}

// 统一取后端 {code, message, data}，失败抛出带后端 message 的 ApiError
export async function api<T>(method: string, url: string, data?: unknown): Promise<T> {
  try {
    const res = await client.request<ApiResponse<T>>({
      method,
      url,
      data: method === "GET" ? undefined : data,
      params: method === "GET" ? data : undefined,
    })
    if (res.data.code !== 0) {
      throw new ApiError(res.data.message, res.data.code, res.status, res.data.data)
    }
    return res.data.data
  } catch (err) {
    if (err instanceof ApiError) throw err
    if (axios.isAxiosError(err)) {
      const body = err.response?.data as Partial<ApiResponse<unknown>> | undefined
      const message = body?.message || (err.code === "ECONNABORTED" ? "网络超时，请稍后再试" : "网络异常，请检查网络")
      throw new ApiError(message, body?.code ?? -1, err.response?.status ?? 0, body?.data)
    }
    throw err
  }
}

export function errorMessage(err: unknown, fallback = "操作失败"): string {
  if (err instanceof Error && err.message) return err.message
  return fallback
}

export function getUploadErrorMessage(err: unknown, fallback = "上传失败"): string {
  if (axios.isAxiosError(err) && err.response?.data?.message) {
    return err.response.data.message
  }
  return errorMessage(err, fallback)
}

export default client
