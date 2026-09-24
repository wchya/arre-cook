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

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

export function setToken(token: string | null) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token)
    else localStorage.removeItem(TOKEN_KEY)
  } catch {
    /* ignore */
  }
}

client.interceptors.request.use((config) => {
  const token = getToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

client.interceptors.response.use(
  (res) => res,
  (err) => {
    const url: string = err.config?.url || ""
    if (err.response?.status === 401 && !url.startsWith("/auth/")) {
      setToken(null)
      window.dispatchEvent(new Event("auth-expired"))
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
