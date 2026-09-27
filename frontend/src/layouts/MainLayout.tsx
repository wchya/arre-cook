import { lazy, Suspense, useState } from "react"
import { NavLink, useLocation, useNavigate } from "react-router-dom"
import { CalendarDays, Home as HomeIcon, Sparkles, UserRound, UtensilsCrossed } from "lucide-react"
import Home from "@/pages/Home"
import RequestState from "@/components/RequestState"

const tabs = [
  { path: "/", Component: Home, icon: HomeIcon, label: "首页", exact: true },
  { path: "/dishes", Component: lazy(() => import("@/pages/DishList")), icon: UtensilsCrossed, label: "菜谱" },
  { path: "/history", Component: lazy(() => import("@/pages/History")), icon: CalendarDays, label: "记录" },
  { path: "/me", Component: lazy(() => import("@/pages/Me")), icon: UserRound, label: "我的" },
]

export default function MainLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const activeIdx = Math.max(0, tabs.findIndex((tab) => tab.path === location.pathname))
  const [visited, setVisited] = useState(() => new Set([activeIdx]))
  if (!visited.has(activeIdx)) setVisited(new Set([...visited, activeIdx]))

  function renderTab(index: number) {
    const tab = tabs[index]
    const Icon = tab.icon
    const active = activeIdx === index
    return <NavLink key={tab.path} to={tab.path} end={tab.exact}
      className={`flex min-h-12 min-w-0 flex-col items-center justify-center gap-1 rounded-2xl text-xs font-semibold transition-colors ${active ? "text-primary" : "text-text2 hover:bg-bg"}`}>
      <span className={`flex h-7 w-11 items-center justify-center rounded-full ${active ? "bg-primary-light" : ""}`}><Icon size={21} strokeWidth={active ? 2.4 : 2} /></span>
      <span>{tab.label}</span>
    </NavLink>
  }

  return (
    <div className="h-dvh overflow-hidden bg-bg">
      <a href="#main-content" className="skip-link">跳到主要内容</a>
      <main id="main-content" tabIndex={-1} className="h-full outline-none">
        {tabs.map((tab, index) => visited.has(index) && (
          <section key={tab.path} hidden={activeIdx !== index} inert={activeIdx !== index}
            aria-label={tab.label} className="app-scroll h-full overflow-y-auto pb-[calc(88px+env(safe-area-inset-bottom))]">
            <Suspense fallback={<RequestState />}><tab.Component /></Suspense>
          </section>
        ))}
      </main>
      <nav aria-label="主导航" className="fixed inset-x-0 bottom-0 z-[100] border-t border-glass-border glass pb-[env(safe-area-inset-bottom)]">
        <div className="mx-auto grid h-[72px] max-w-[640px] grid-cols-5 items-center gap-1 px-3">
          {renderTab(0)}{renderTab(1)}
          <button type="button" onClick={() => navigate("/assistant/chat")} aria-label="打开 AI 食谱助手"
            className="mx-auto flex h-14 w-14 flex-col items-center justify-center gap-1 rounded-[20px] bg-primary text-white shadow-sm transition-transform active:scale-95">
            <Sparkles size={22} /><span className="text-[10px] font-semibold">AI 助手</span>
          </button>
          {renderTab(2)}{renderTab(3)}
        </div>
      </nav>
    </div>
  )
}
