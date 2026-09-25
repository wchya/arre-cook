import { NavLink, useLocation, useNavigate } from "react-router-dom"
import { useEffect, useRef } from "react"
import { CalendarDays, Home as HomeIcon, Sparkles, UserRound, UtensilsCrossed } from "lucide-react"
import { gsap, motionDuration, useGSAP } from "@/lib/gsap"
import Home from "@/pages/Home"
import DishList from "@/pages/DishList"
import History from "@/pages/History"
import Me from "@/pages/Me"

const tabs = [
  { path: "/", Component: Home, icon: HomeIcon, label: "首页", exact: true },
  { path: "/dishes", Component: DishList, icon: UtensilsCrossed, label: "菜谱" },
  { path: "/history", Component: History, icon: CalendarDays, label: "记录" },
  { path: "/me", Component: Me, icon: UserRound, label: "我的" },
]

const TAB_COUNT = tabs.length
const STEP_PCT = 100 / TAB_COUNT

const EDGE_ZONE = 36
const SWIPE_MIN = 30

export default function MainLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const activeIdx = Math.max(0, tabs.findIndex((t) => t.path === location.pathname))
  const stripRef = useRef<HTMLDivElement>(null)
  const didMountRef = useRef(false)
  const animatingRef = useRef(false)
  const offsetXRef = useRef(0)
  const touchRef = useRef<{
    startX: number
    startY: number
    startEdge: "left" | "right"
    directionDecided: boolean
    isHorizontal: boolean | null
  } | null>(null)

  useGSAP(() => {
    const el = stripRef.current
    if (!el) return
    const duration = didMountRef.current ? motionDuration(0.32) : 0
    didMountRef.current = true
    offsetXRef.current = 0
    animatingRef.current = duration > 0
    gsap.to(el, {
      xPercent: -activeIdx * STEP_PCT,
      x: 0,
      duration,
      ease: "power3.out",
      force3D: true,
      onComplete: () => {
        animatingRef.current = false
      },
    })
  }, { dependencies: [activeIdx], scope: stripRef })

  // 屏幕边缘左右滑动切换 Tab
  useEffect(() => {
    function onTouchStart(e: TouchEvent) {
      if (animatingRef.current) return
      const t = e.touches[0]
      const x = t.clientX
      const vw = window.innerWidth
      let startEdge: "left" | "right" | null = null
      if (x < EDGE_ZONE) startEdge = "left"
      else if (x > vw - EDGE_ZONE) startEdge = "right"
      if (!startEdge) return
      if (startEdge === "left" && activeIdx === 0) return
      if (startEdge === "right" && activeIdx === tabs.length - 1) return
      touchRef.current = { startX: x, startY: t.clientY, startEdge, directionDecided: false, isHorizontal: null }
    }

    function resetStrip() {
      touchRef.current = null
      offsetXRef.current = 0
      gsap.to(stripRef.current, { x: 0, duration: motionDuration(0.18), ease: "power2.out" })
    }

    function onTouchMove(e: TouchEvent) {
      const ref = touchRef.current
      if (!ref) return
      const dx = e.touches[0].clientX - ref.startX
      const dy = e.touches[0].clientY - ref.startY
      if (!ref.directionDecided) {
        if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return
        ref.directionDecided = true
        ref.isHorizontal = Math.abs(dx) > Math.abs(dy)
      }
      if (!ref.isHorizontal) return resetStrip()
      if ((ref.startEdge === "left" && dx < 0) || (ref.startEdge === "right" && dx > 0)) return resetStrip()

      // Only claim a confirmed horizontal edge gesture. Vertical gestures must
      // remain native scrolling gestures, especially in Android WebView.
      e.preventDefault()
      const vw = window.innerWidth
      const maxDx = vw * 0.35
      const clampedDx = Math.max(-maxDx, Math.min(maxDx, dx))
      const rubber = clampedDx * (1 - Math.abs(clampedDx) / (vw * 1.2))
      offsetXRef.current = rubber
      gsap.set(stripRef.current, { x: rubber, force3D: true })
    }

    function onTouchEnd() {
      const ref = touchRef.current
      if (!ref) return
      const dx = offsetXRef.current
      if (ref.startEdge === "left" && dx > SWIPE_MIN && activeIdx > 0) {
        offsetXRef.current = 0
        navigate(tabs[activeIdx - 1].path)
      } else if (ref.startEdge === "right" && dx < -SWIPE_MIN && activeIdx < tabs.length - 1) {
        offsetXRef.current = 0
        navigate(tabs[activeIdx + 1].path)
      } else if (stripRef.current) {
        offsetXRef.current = 0
        animatingRef.current = true
        gsap.to(stripRef.current, {
          xPercent: -activeIdx * STEP_PCT,
          x: 0,
          duration: motionDuration(0.25),
          ease: "power3.out",
          onComplete: () => {
            animatingRef.current = false
          },
        })
      }
      touchRef.current = null
    }

    document.addEventListener("touchstart", onTouchStart, { passive: true })
    document.addEventListener("touchmove", onTouchMove, { passive: false })
    document.addEventListener("touchend", onTouchEnd, { passive: true })
    document.addEventListener("touchcancel", onTouchEnd, { passive: true })
    return () => {
      document.removeEventListener("touchstart", onTouchStart)
      document.removeEventListener("touchmove", onTouchMove)
      document.removeEventListener("touchend", onTouchEnd)
      document.removeEventListener("touchcancel", onTouchEnd)
    }
  }, [activeIdx, navigate])

  const goTab = (idx: number) => {
    if (idx !== activeIdx) navigate(tabs[idx].path)
  }

  const renderTab = (idx: number) => {
    const tab = tabs[idx]
    const Icon = tab.icon
    const isActive = activeIdx === idx
    return (
      <NavLink
        key={tab.path}
        to={tab.path}
        end={tab.exact}
        onClick={(e) => {
          e.preventDefault()
          goTab(idx)
        }}
        className={`group relative flex h-12 min-w-0 flex-col items-center justify-center gap-0.5 rounded-[13px] text-[10px] font-semibold transition-all active:scale-95 ${isActive ? "text-primary" : "text-text3 hover:text-text2"}`}
      >
        <span className={`flex h-6 w-9 items-center justify-center rounded-full transition-all ${isActive ? "bg-primary-light text-primary" : "text-text3 group-hover:bg-bg"}`}>
          <Icon size={19} strokeWidth={isActive ? 2.6 : 2.1} />
        </span>
        <span className={`max-w-full truncate leading-none transition-colors ${isActive ? "text-primary" : "text-text3"}`}>{tab.label}</span>
      </NavLink>
    )
  }

  return (
    <div className="h-dvh overflow-hidden bg-bg" style={{ touchAction: "pan-y" }}>
      <div className="h-full overflow-hidden">
        <div ref={stripRef} className="flex h-full will-change-transform" style={{ width: `${TAB_COUNT * 100}%` }}>
          {tabs.map((tab) => (
            <div
              key={tab.path}
              className="app-scroll h-full overflow-y-auto overscroll-y-contain pb-[calc(68px+env(safe-area-inset-bottom))]"
              style={{ width: `${STEP_PCT}%`, flexShrink: 0 }}
            >
              <tab.Component />
            </div>
          ))}
        </div>
      </div>

      <nav
        className="fixed bottom-0 left-0 right-0 z-[100] border-t border-border/70 bg-card/92 shadow-[0_-10px_30px_rgba(26,26,46,.07)] backdrop-blur-2xl"
        style={{ paddingBottom: "env(safe-area-inset-bottom)" }}
      >
        <div className="mx-auto grid h-[60px] max-w-[640px] grid-cols-5 items-center px-2">
          {renderTab(0)}
          {renderTab(1)}
          <div className="flex justify-center">
            <button
              onClick={() => navigate("/assistant/chat")}
              aria-label="AI 食谱助手"
              className="relative -mt-7 flex h-[58px] w-[58px] flex-col items-center justify-center rounded-[22px] bg-gradient-to-br from-[#F59A6B] via-primary to-[#D9573A] text-white shadow-[0_12px_28px_rgba(232,115,74,.45)] ring-4 ring-bg transition-all active:scale-95"
            >
              <Sparkles size={24} strokeWidth={2.3} />
              <span className="mt-0.5 text-[9px] font-extrabold leading-none tracking-wide">AI 助手</span>
            </button>
          </div>
          {renderTab(2)}
          {renderTab(3)}
        </div>
      </nav>
    </div>
  )
}
