import { useEffect, useRef } from "react"
import { createPortal } from "react-dom"
import { gsap, motionDuration, useGSAP } from "@/lib/gsap"
import { useDialog } from "@/lib/use-dialog"

type BottomSheetControls = { close: () => void }

interface AnimatedBottomSheetProps {
  children: React.ReactNode | ((controls: BottomSheetControls) => React.ReactNode)
  onClose: () => void
  className?: string
  zIndexClass?: string
  label?: string
}

export default function AnimatedBottomSheet({
  children,
  onClose,
  className = "",
  zIndexClass = "z-[200]",
  label = "操作面板",
}: AnimatedBottomSheetProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const sheetRef = useRef<HTMLDivElement>(null)
  const closingRef = useRef(false)

  const { contextSafe } = useGSAP(() => {
    const tl = gsap.timeline({ defaults: { ease: "power3.out" } })
    tl.fromTo(rootRef.current, { autoAlpha: 0 }, { autoAlpha: 1, duration: motionDuration(0.18) })
      .fromTo(
        sheetRef.current,
        { yPercent: 100 },
        { yPercent: 0, duration: motionDuration(0.34), clearProps: "transform" },
        0,
      )
  }, { scope: rootRef })

  // GSAP's contextSafe wrapper invokes this only after the sheet has mounted.
  // eslint-disable-next-line react-hooks/refs
  const close = contextSafe(() => {
    if (closingRef.current) return
    closingRef.current = true
    gsap.timeline({ onComplete: onClose, defaults: { ease: "power2.in" } })
      .to(sheetRef.current, { yPercent: 100, duration: motionDuration(0.24) }, 0)
      .to(rootRef.current, { autoAlpha: 0, duration: motionDuration(0.18) }, 0)
  })

  useDialog(sheetRef, close)
  useEffect(() => {
    const viewport = window.visualViewport
    const root = rootRef.current
    if (!viewport || !root) return
    const resize = () => {
      root.style.height = `${viewport.height}px`
      root.style.top = `${viewport.offsetTop}px`
    }
    resize()
    viewport.addEventListener("resize", resize)
    viewport.addEventListener("scroll", resize)
    return () => { viewport.removeEventListener("resize", resize); viewport.removeEventListener("scroll", resize) }
  }, [])

  return createPortal(
    <div ref={rootRef} data-dialog-root className={`fixed inset-0 ${zIndexClass} flex items-end justify-center`} onClick={close}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-[3px]" />
      <div
        ref={sheetRef}
        role="dialog"
        aria-modal="true"
        aria-label={label}
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        className={`app-sheet relative w-full max-w-[640px] border-t border-border bg-card shadow-[0_-12px_40px_rgba(26,26,46,.14)] will-change-transform ${className}`}
      >
        {typeof children === "function" ? children({ close }) : children}
      </div>
    </div>,
    document.body,
  )
}
