import type { ReactNode } from "react"
import { Navigate, useLocation } from "react-router-dom"
import { useAuthStore } from "@/store/useAuthStore"
import AchievementUnlockOverlay from "@/components/AchievementUnlockOverlay"
import Splash from "@/components/Splash"

// 登录门禁：未登录跳到 /login 并带上回跳地址
export default function RequireAuth({ children, adminOnly = false }: { children: ReactNode; adminOnly?: boolean }) {
  const status = useAuthStore((s) => s.status)
  const isAdmin = useAuthStore((s) => s.user?.role === "admin")
  const location = useLocation()

  if (status === "unknown") return <Splash />
  if (status === "anonymous") {
    const redirect = encodeURIComponent(location.pathname + location.search)
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
