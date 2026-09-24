import { NavLink, Outlet, useNavigate } from "react-router-dom"
import { ArrowLeft, ChartNoAxesCombined, ChefHat, LogOut, Settings2, Trophy, Users } from "lucide-react"
import { useAuthStore } from "@/store/useAuthStore"
import { useAppInfoStore } from "@/store/useAppInfoStore"

const navItems = [
  { path: "/admin/dashboard", label: "概览", Icon: ChartNoAxesCombined },
  { path: "/admin/dishes", label: "菜谱", Icon: ChefHat },
  { path: "/admin/users", label: "用户", Icon: Users },
  { path: "/admin/achievements", label: "成就", Icon: Trophy },
  { path: "/admin/quotes", label: "语录", Icon: Settings2 },
  { path: "/admin/settings", label: "设置", Icon: Settings2 },
]

export default function AdminLayout() {
  const navigate = useNavigate()
  const logout = useAuthStore((s) => s.logout)
  const appName = useAppInfoStore((s) => s.appName)

  return (
    <div className="flex h-dvh flex-col overflow-hidden bg-bg">
      <header className="flex shrink-0 items-center gap-3 border-b border-border bg-card px-4 py-3">
        <button onClick={() => navigate("/")} aria-label="返回首页" title="返回首页" className="flex h-9 w-9 items-center justify-center rounded-lg text-text2 hover:bg-bg focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">
          <ArrowLeft size={18} />
        </button>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-bold">{appName} 管理</div>
          <div className="text-[11px] text-text3">站点与账号管理</div>
        </div>
        <button onClick={() => { void logout(); navigate("/") }} aria-label="退出登录" title="退出登录" className="flex h-9 w-9 items-center justify-center rounded-lg text-text2 hover:bg-bg focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">
          <LogOut size={17} />
        </button>
      </header>
      <nav aria-label="管理导航" className="shrink-0 border-b border-border bg-card">
        <div className="mx-auto flex max-w-[960px] gap-1 overflow-x-auto px-3 py-2">
          {navItems.map(({ path, label, Icon }) => (
            <NavLink
              key={path}
              to={path}
              className={({ isActive }) => `flex h-9 shrink-0 items-center gap-2 rounded-lg px-3 text-xs font-semibold transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary ${isActive ? "bg-text text-white" : "text-text2 hover:bg-bg"}`}
            >
              <Icon size={15} />
              {label}
            </NavLink>
          ))}
        </div>
      </nav>
      <main className="min-h-0 flex-1 overflow-y-auto overscroll-y-contain">
        <Outlet />
      </main>
    </div>
  )
}
