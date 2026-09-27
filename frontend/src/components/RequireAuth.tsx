import type { ReactNode } from "react"
import { Navigate, useLocation } from "react-router-dom"
import { useAuthStore } from "@/store/useAuthStore"
import AchievementUnlockOverlay from "@/components/AchievementUnlockOverlay"
import Splash from "@/components/Splash"
import RequestState from "@/components/RequestState"

// 登录门禁：未登录跳到 /login 并带上回跳地址
export default function RequireAuth({ children, adminOnly = false }: { children: ReactNode; adminOnly?: boolean }) {
  const status = useAuthStore((s) => s.status)
  const bootstrap = useAuthStore((s) => s.bootstrap)
  const isAdmin = useAuthStore((s) => s.user?.role === "admin")
  const location = useLocation()

  if (status === "unknown") return <Splash />
  if (status === "unavailable") return <RequestState error={new Error("暂时无法连接服务器，登录信息已保留，请检查网络后重试。")} onRetry={() => { void bootstrap() }} />
  if (status === "anonymous") {
    const redirect = encodeURIComponent(location.pathname + location.search + location.hash)
    return <Navigate to={`/login?redirect=${redirect}`} replace />
  }
  if (adminOnly && !isAdmin) return <Navigate to="/" replace />
  return (
    <>
      {children}
      <AchievementUnlockOverlay />
    </>
  )
}
