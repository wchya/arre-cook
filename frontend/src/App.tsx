import { lazy, Suspense, useEffect } from "react"
import { BrowserRouter, Navigate, Route, Routes, useLocation } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Toaster } from "react-hot-toast"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import { useAuthStore } from "@/store/useAuthStore"
import RequireAuth from "@/components/RequireAuth"
import Splash from "@/components/Splash"
import MainLayout from "@/layouts/MainLayout"
import SubPageLayout from "@/layouts/SubPageLayout"
import Login from "@/pages/Login"
import { setMiniProgramShare } from "@/lib/miniprogram"

// 子页面与管理后台按需加载，首屏只带首页相关代码
const DishDetail = lazy(() => import("@/pages/DishDetail"))
const DishEditor = lazy(() => import("@/pages/admin/DishEdit"))
const Favorites = lazy(() => import("@/pages/Favorites"))
const PhotoWall = lazy(() => import("@/pages/PhotoWall"))
const Plan = lazy(() => import("@/pages/More"))
const Tomorrow = lazy(() => import("@/pages/Tomorrow"))
const Assistant = lazy(() => import("@/pages/Assistant"))
const TasteProfile = lazy(() => import("@/pages/TasteProfile"))
const Suggestions = lazy(() => import("@/pages/Suggestions"))
const Achievements = lazy(() => import("@/pages/Achievements"))
const CookMode = lazy(() => import("@/pages/CookMode"))
const Preferences = lazy(() => import("@/pages/Preferences"))
const AiConnections = lazy(() => import("@/pages/AiConnections"))
const Account = lazy(() => import("@/pages/Account"))
const Family = lazy(() => import("@/pages/Family"))
const Health = lazy(() => import("@/pages/Health"))
const AdminLayout = lazy(() => import("@/layouts/AdminLayout"))
const AdminDashboard = lazy(() => import("@/pages/admin/Dashboard"))
const AdminDishes = lazy(() => import("@/pages/admin/Dishes"))
const AdminUsers = lazy(() => import("@/pages/admin/Users"))
const AdminSettings = lazy(() => import("@/pages/admin/Settings"))
const AdminAchievements = lazy(() => import("@/pages/admin/Achievements"))
const AdminQuotes = lazy(() => import("@/pages/admin/Quotes"))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30000,
      retry: (count, err) => {
        const status = (err as { status?: number })?.status
        if (status === 401 || status === 403 || status === 404) return false
        return count < 1
      },
    },
  },
})

// 切换账号 / 退出时清空缓存，避免上一个用户的数据残留在页面上
let lastUserId: number | null | undefined
useAuthStore.subscribe((s) => {
  const id = s.user?.id ?? null
  if (lastUserId !== undefined && id !== lastUserId) queryClient.clear()
  lastUserId = id
})

function MiniProgramBridge() {
  const location = useLocation()
  const status = useAuthStore((state) => state.status)
  const appName = useAppInfoStore((state) => state.appName)
  useEffect(() => {
    void setMiniProgramShare(`${appName} · 今天吃什么`, `${location.pathname}${location.search}`)
  }, [appName, location.pathname, location.search, status])
  return null
}

export default function App() {
  const fetchAppInfo = useAppInfoStore((s) => s.fetch)
  const appName = useAppInfoStore((s) => s.appName)
  const bootstrap = useAuthStore((s) => s.bootstrap)

  useEffect(() => {
    void fetchAppInfo()
    void bootstrap()
  }, [fetchAppInfo, bootstrap])
  useEffect(() => {
    document.title = appName
  }, [appName])

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <MiniProgramBridge />
        <Suspense fallback={<Splash />}>
          <Routes>
            <Route path="/login" element={<Login />} />

            <Route element={<RequireAuth><MainLayout /></RequireAuth>}>
              <Route path="/" element={null} />
              <Route path="/dishes" element={null} />
              <Route path="/history" element={null} />
              <Route path="/me" element={null} />
            </Route>

            <Route element={<RequireAuth><SubPageLayout /></RequireAuth>}>
              <Route path="/dishes/new" element={<DishEditor mode="user" />} />
              <Route path="/dishes/:id" element={<DishDetail />} />
              <Route path="/dishes/:id/edit" element={<DishEditor mode="user" />} />
              <Route path="/favorites" element={<Favorites />} />
              <Route path="/photo-wall" element={<PhotoWall />} />
              <Route path="/plan" element={<Plan />} />
              <Route path="/tomorrow" element={<Tomorrow />} />
              <Route path="/taste-profile" element={<TasteProfile />} />
              <Route path="/suggestions" element={<Suggestions />} />
              <Route path="/achievements" element={<Achievements />} />
              <Route path="/me/preferences" element={<Preferences />} />
              <Route path="/me/ai" element={<AiConnections />} />
              <Route path="/me/account" element={<Account />} />
              <Route path="/family" element={<Family />} />
              <Route path="/health" element={<Health />} />
            </Route>

            <Route path="/assistant" element={<RequireAuth><Assistant /></RequireAuth>} />
            <Route path="/dishes/:id/cook" element={<RequireAuth><CookMode /></RequireAuth>} />

            <Route path="/admin" element={<RequireAuth adminOnly><AdminLayout /></RequireAuth>}>
              <Route index element={<Navigate to="/admin/dashboard" replace />} />
              <Route path="dashboard" element={<AdminDashboard />} />
              <Route path="dishes" element={<AdminDishes />} />
              <Route path="dishes/new" element={<DishEditor mode="admin" />} />
              <Route path="dishes/:id" element={<DishEditor mode="admin" />} />
              <Route path="users" element={<AdminUsers />} />
              <Route path="settings" element={<AdminSettings />} />
              <Route path="achievements" element={<AdminAchievements />} />
              <Route path="quotes" element={<AdminQuotes />} />
            </Route>

            {/* 旧地址兼容 */}
            <Route path="/admin/login" element={<Navigate to="/login?redirect=/admin/dashboard" replace />} />
            <Route path="/admin/records" element={<Navigate to="/history" replace />} />
            <Route path="/more" element={<Navigate to="/plan" replace />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </BrowserRouter>
      <Toaster
        position="top-center"
        containerStyle={{ top: "calc(12px + env(safe-area-inset-top))" }}
        toastOptions={{
          duration: 2200,
          style: {
            borderRadius: "9999px",
            background: "var(--color-card)",
            color: "var(--color-text)",
            fontSize: "14px",
            padding: "10px 22px",
            boxShadow: "0 10px 34px rgba(0,0,0,.12)",
          },
        }}
      />
    </QueryClientProvider>
  )
}
