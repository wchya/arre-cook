import { create } from "zustand"
import { meApi } from "@/api"
import { ApiError, getToken, setToken } from "@/api/client"
import { backToMiniProgramLogin, consumeTokenFromURL, isMiniProgram } from "@/lib/miniprogram"
import type { User } from "@/types"

type Status = "unknown" | "authenticated" | "anonymous" | "unavailable"

interface AuthState {
  status: Status
  isLoggedIn: boolean
  user: User | null
  bootstrap: () => Promise<void>
  setSession: (token: string, user: User) => void
  setUser: (user: User) => void
  refresh: () => Promise<void>
  logout: (opts?: { expired?: boolean }) => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  status: "unknown",
  isLoggedIn: false,
  user: null,

  // 启动：先吃掉小程序带来的令牌，再用令牌换当前用户
  bootstrap: async () => {
    const fromURL = consumeTokenFromURL()
    if (fromURL) setToken(fromURL)
    const startedWith = getToken()
    if (!startedWith) {
      set({ status: "anonymous", isLoggedIn: false, user: null })
      return
    }
    set({ status: "unknown" })
    try {
      const user = await meApi.get()
      if (getToken() !== startedWith) return
      set({ status: "authenticated", isLoggedIn: true, user })
    } catch (error) {
      if (getToken() !== startedWith) {
        if (!getToken()) set({ status: "anonymous", isLoggedIn: false, user: null })
        return
      }
      if (error instanceof ApiError && error.status === 401) {
        setToken(null)
        set({ status: "anonymous", isLoggedIn: false, user: null })
      } else {
        set({ status: "unavailable", isLoggedIn: false, user: null })
      }
    }
  },

  setSession: (token, user) => {
    setToken(token)
    set({ status: "authenticated", isLoggedIn: true, user })
  },

  setUser: (user) => set({ user }),

  refresh: async () => {
    const startedWith = getToken()
    if (!startedWith) return
    try {
      const user = await meApi.get()
      if (getToken() !== startedWith) return
      set({ user, status: "authenticated", isLoggedIn: true })
    } catch {
      /* 401 会触发 auth-expired */
    }
  },

  logout: async (opts) => {
    setToken(null)
    set({ status: "anonymous", isLoggedIn: false, user: null })
    if (isMiniProgram()) {
      await backToMiniProgramLogin(opts?.expired ? "expired" : "logout")
    }
  },
}))

export const useIsAdmin = () => useAuthStore((s) => s.user?.role === "admin")

// 兼容旧组件里的 isLoggedIn 判断
export const useIsLoggedIn = () => useAuthStore((s) => s.isLoggedIn)

if (typeof window !== "undefined") {
  window.addEventListener("auth-expired", () => {
    if (useAuthStore.getState().status === "authenticated") {
      void useAuthStore.getState().logout({ expired: true })
    }
  })
}

// 供非 React 代码读取
export const currentUser = () => useAuthStore.getState().user
export const isAuthenticated = () => useAuthStore.getState().status === "authenticated"
export { getToken }
