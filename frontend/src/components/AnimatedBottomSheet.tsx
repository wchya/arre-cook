import { useRef } from "react"
import { createPortal } from "react-dom"
import { gsap, motionDuration, useGSAP } from "@/lib/gsap"

type BottomSheetControls = { close: () => void }

interface AnimatedBottomSheetProps {
  children: React.ReactNode | ((controls: BottomSheetControls) => React.ReactNode)
  onClose: () => void
  className?: string
  zIndexClass?: string
}

export default function AnimatedBottomSheet({
  children,
  onClose,
  className = "",
  zIndexClass = "z-[200]",
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

  return createPortal(
    <div ref={rootRef} className={`fixed inset-0 ${zIndexClass} flex items-end justify-center`} onClick={close}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div
        ref={sheetRef}
        onClick={(e) => e.stopPropagation()}
        className={`relative w-full max-w-[640px] bg-card will-change-transform ${className}`}
      >
        {typeof children === "function" ? children({ close }) : children}
      </div>
    </div>,
    document.body,
  )
}
