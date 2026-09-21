import { useEffect, useState } from "react"
import { Sparkles, X } from "lucide-react"

/**
 * 悬浮 AI 助手：以 iframe 内嵌 agent.arrebyte.top 的去壳对话页（/embed）。
 * - 受信来源由 agent 前端 Nginx 的 CSP frame-ancestors 放行（含 https://cook.arrebyte.top）。
 * - 访客无登录态，嵌入页自行在其 localStorage 生成/缓存 guest userId。
 * - 关闭态仅显示右下角悬浮球；展开态：移动端近全屏，桌面端右下角定宽卡片。
 */

// agent 去壳嵌入页；agentId=100003 为当前装配的单一智能体。
const EMBED_SRC =
  "https://agent.arrebyte.top/embed?agentId=100003&title=食谱小助手&mode=light&accent=amber"

export default function AiAssistant() {
  const [open, setOpen] = useState(false)

  // 展开时锁定父页滚动（移动端近全屏，避免背景跟随滚动）
  useEffect(() => {
    if (!open) return
    const prev = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  return (
    <>
      {/* 悬浮球：关闭态可见，位于底部导航（z-100）之上 */}
      {!open && (
        <button
          type="button"
          aria-label="打开食谱小助手"
          onClick={() => setOpen(true)}
          className="fixed right-4 z-[110] flex h-14 w-14 items-center justify-center rounded-full bg-primary text-white shadow-[0_10px_30px_rgba(232,115,74,.45)] transition-all active:scale-95 hover:bg-primary-dark"
          style={{ bottom: "calc(76px + env(safe-area-inset-bottom))" }}
        >
          <Sparkles size={24} strokeWidth={2.2} />
        </button>
      )}

      {/* 对话面板 */}
      {open && (
        <>
          {/* 遮罩：点击关闭（桌面端透明、仅承载点击） */}
          <div
            className="fixed inset-0 z-[110] bg-black/30 backdrop-blur-[2px] sm:bg-transparent sm:backdrop-blur-none"
            onClick={() => setOpen(false)}
          />
          <div
            role="dialog"
            aria-label="食谱小助手"
            className="fixed z-[111] overflow-hidden rounded-[18px] bg-white shadow-[0_20px_60px_rgba(0,0,0,.30)] inset-x-3 bottom-3 top-16 sm:inset-auto sm:right-5 sm:bottom-5 sm:top-auto sm:h-[560px] sm:w-[384px]"
          >
            <button
              type="button"
              aria-label="关闭食谱小助手"
              onClick={() => setOpen(false)}
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
              loading="lazy"
            />
          </div>
        </>
      )}
    </>
  )
}
