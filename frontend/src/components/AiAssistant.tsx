import { useEffect, useRef, useState } from "react"
import { Sparkles, X } from "lucide-react"

/**
 * 悬浮 AI 助手：以 iframe 内嵌 agent.arrebyte.top 的去壳对话页（/embed）。
 * - 受信来源由 agent 前端 Nginx 的 CSP frame-ancestors 放行（含 https://cook.arrebyte.top）。
 * - 访客无登录态，嵌入页自行在其 localStorage 生成/缓存 guest userId。
 * - 关闭态仅显示右下角悬浮球；展开态：移动端近全屏，桌面端右下角定宽卡片。
 * - 首次到访：悬浮球带呼吸光环 + 未读点，并弹出一句引导气泡；用户点开过一次后
 *   （localStorage 记忆）不再打扰。开合走过渡动画，收起也有回弹，手感更顺。
 */

// agent 去壳嵌入页；agentId=100003 为当前装配的单一智能体。
const EMBED_SRC =
  "https://agent.arrebyte.top/embed?agentId=100003&title=食谱小助手&mode=auto&accent=amber"

// 记忆用户是否已打开过助手：打开过就不再显示引导气泡/光环。
const SEEN_KEY = "nini-ai-assistant-seen"

export default function AiAssistant() {
  const [open, setOpen] = useState(false)   // 是否渲染面板（含收起动画期间）
  const [shown, setShown] = useState(false) // 面板可见态，驱动进/出过渡
  // 首次打开后常驻 iframe（收起只隐藏）：避免每次打开都重载嵌入页造成闪烁，也保留对话上下文。
  const [mounted, setMounted] = useState(false)
  const [seen, setSeen] = useState(() => {
    try {
      return localStorage.getItem(SEEN_KEY) === "1"
    } catch {
      return true
    }
  })
  const [showHint, setShowHint] = useState(false) // 首次引导气泡
  const closeTimer = useRef<number | null>(null)
  const hintTimer = useRef<number | null>(null)

  // 首帧读取"是否已见过"。未见过则稍延迟弹出引导气泡（等页面稳定，避免与入场动画打架）。
  useEffect(() => {
    if (seen) return
    hintTimer.current = window.setTimeout(() => setShowHint(true), 1200)
    const dismissTimer = window.setTimeout(() => setShowHint(false), 1200 + 8000)
    return () => {
      if (hintTimer.current) window.clearTimeout(hintTimer.current)
      window.clearTimeout(dismissTimer)
      if (closeTimer.current) window.clearTimeout(closeTimer.current)
    }
  }, [seen])

  // 展开时锁定父页滚动（移动端近全屏，避免背景跟随滚动）。
  useEffect(() => {
    if (!open) return
    const prev = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  const markSeen = () => {
    setSeen(true)
    setShowHint(false)
    try {
      localStorage.setItem(SEEN_KEY, "1")
    } catch {
      /* 隐私模式等写入失败可忽略，仅影响下次是否再引导 */
    }
  }

  const openPanel = () => {
    markSeen()
    if (closeTimer.current) {
      window.clearTimeout(closeTimer.current)
      closeTimer.current = null
    }
    setOpen(true)
    setMounted(true)
    // 下一帧再切到可见态，让过渡从初始态起跳。
    requestAnimationFrame(() => setShown(true))
  }

  const closePanel = () => {
    setShown(false)
    // 等收起过渡跑完再卸载面板（与下方 duration-200 对齐）。
    closeTimer.current = window.setTimeout(() => setOpen(false), 200)
  }

  return (
    <>
      {/* 悬浮球：关闭态可见，位于底部导航（z-100）之上 */}
      {!open && (
        <>
          {/* 首次引导气泡：点它也能打开助手 */}
          {showHint && (
            <button
              type="button"
              onClick={openPanel}
              className="animate-pop-soft fixed right-4 z-[111] max-w-[220px] rounded-2xl rounded-br-md bg-card px-3.5 py-2.5 text-left text-sm leading-snug text-text shadow-[0_12px_34px_rgba(0,0,0,.16)] ring-1 ring-black/5"
              style={{ bottom: "calc(140px + env(safe-area-inset-bottom))" }}
            >
              <span className="font-medium text-primary">今天吃什么？</span>
              <span className="text-text2"> 问我就好 👋</span>
              <span
                role="button"
                aria-label="关闭提示"
                onClick={(e) => {
                  e.stopPropagation()
                  setShowHint(false)
                }}
                className="absolute -right-1.5 -top-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-text2/90 text-bg shadow-sm"
              >
                <X size={12} strokeWidth={2.6} />
              </span>
            </button>
          )}

          <button
            type="button"
            aria-label="打开食谱小助手"
            onClick={openPanel}
            className="fixed right-4 z-[110] flex h-14 w-14 items-center justify-center rounded-full bg-primary text-white shadow-[0_10px_30px_rgba(232,115,74,.45)] transition-all active:scale-95 hover:bg-primary-dark"
            style={{ bottom: "calc(76px + env(safe-area-inset-bottom))" }}
          >
            {/* 首次到访的呼吸光环（未读引导），点开一次后不再显示 */}
            {!seen && (
              <span className="animate-glow pointer-events-none absolute inset-0 rounded-full" />
            )}
            <Sparkles size={24} strokeWidth={2.2} />
            {/* 未读点 */}
            {!seen && (
              <span className="absolute right-1 top-1 h-3 w-3 rounded-full border-2 border-white bg-red-500" />
            )}
          </button>
        </>
      )}

      {/* 遮罩：点击关闭（桌面端透明、仅承载点击），随面板淡入淡出 */}
      {open && (
        <div
          className={`fixed inset-0 z-[110] bg-black/30 backdrop-blur-[2px] transition-opacity duration-200 sm:bg-transparent sm:backdrop-blur-none ${
            shown ? "opacity-100" : "opacity-0"
          }`}
          onClick={closePanel}
        />
      )}

      {/* 对话面板：首次打开后常驻，收起时隐藏而非卸载 */}
      {mounted && (
        <div
          role="dialog"
          aria-label="食谱小助手"
          aria-hidden={!open}
          className={`fixed z-[111] overflow-hidden rounded-[18px] bg-card shadow-[0_20px_60px_rgba(0,0,0,.30)] transition-all duration-200 ease-out inset-x-3 bottom-3 top-16 sm:inset-auto sm:right-5 sm:bottom-5 sm:top-auto sm:h-[560px] sm:w-[384px] sm:origin-bottom-right ${
            shown
              ? "opacity-100 translate-y-0 scale-100"
              : "opacity-0 translate-y-3 scale-95 sm:translate-y-2"
          } ${open ? "" : "invisible pointer-events-none"}`}
        >
          <button
            type="button"
            aria-label="关闭食谱小助手"
            onClick={closePanel}
            className="absolute right-2.5 top-2.5 z-10 flex h-8 w-8 items-center justify-center rounded-full bg-black/10 text-text2 backdrop-blur transition-all active:scale-90 hover:bg-black/20"
          >
            <X size={18} strokeWidth={2.4} />
          </button>
          <iframe
            src={EMBED_SRC}
            title="食谱小助手"
            className="h-full w-full border-0 bg-transparent"
            allow="clipboard-write"
            referrerPolicy="strict-origin-when-cross-origin"
          />
        </div>
      )}
    </>
  )
}
