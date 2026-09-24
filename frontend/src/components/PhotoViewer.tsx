import { useEffect, useRef } from "react"
import { gsap, motionDuration, useGSAP } from "@/lib/gsap"

interface PhotoViewerProps {
  photos: string[]
  idx: number
  setIdx: (i: number) => void
  onClose: () => void
}

export default function PhotoViewer({ photos, idx, setIdx, onClose }: PhotoViewerProps) {
  const layerRef = useRef<HTMLDivElement>(null)
  const stripRef = useRef<HTMLDivElement>(null)
  const startX = useRef(0)
  const animating = useRef(false)
  const idxRef = useRef(idx)

  useEffect(() => {
    idxRef.current = idx
    gsap.set(stripRef.current, { x: -idx * window.innerWidth })
  }, [idx])

  const { contextSafe } = useGSAP(() => {
    gsap.set(stripRef.current, { x: -idxRef.current * window.innerWidth })
    gsap.fromTo(layerRef.current, { autoAlpha: 0 }, { autoAlpha: 1, duration: motionDuration(0.2) })
  }, { scope: layerRef })

  // GSAP contextSafe callbacks run from user events after the viewer has mounted.
  // eslint-disable-next-line react-hooks/refs
  const close = contextSafe(() => {
    gsap.to(layerRef.current, {
      autoAlpha: 0,
      duration: motionDuration(0.18),
      ease: "power1.out",
      onComplete: onClose,
    })
  })

  // eslint-disable-next-line react-hooks/refs
  const snapTo = contextSafe((newIdx: number, animate: boolean) => {
    if (!stripRef.current) return
    animating.current = true
    gsap.to(stripRef.current, {
      x: -newIdx * window.innerWidth,
      duration: animate ? motionDuration(0.3) : 0,
      ease: "power3.out",
      overwrite: true,
      onComplete: () => {
        animating.current = false
        idxRef.current = newIdx
        setIdx(newIdx)
      },
    })
  })

  function applyTransform(offset: number) {
    if (!stripRef.current) return
    gsap.set(stripRef.current, {
      x: -idxRef.current * window.innerWidth + offset,
      force3D: true,
    })
  }

  function onTouchStart(e: React.TouchEvent) {
    if (animating.current) return
    startX.current = e.touches[0].clientX
  }

  function onTouchMove(e: React.TouchEvent) {
    if (animating.current) return
    const diff = e.touches[0].clientX - startX.current
    const damping = photos.length <= 1 ? 0.2 : 1
    applyTransform(diff * damping)
  }

  function onTouchEnd(e: React.TouchEvent) {
    if (animating.current) return
    const diff = e.changedTouches[0].clientX - startX.current
    if (diff < -80 && photos.length > 1) snapTo((idxRef.current + 1) % photos.length, true)
    else if (diff > 80 && photos.length > 1) snapTo((idxRef.current - 1 + photos.length) % photos.length, true)
    else snapTo(idxRef.current, true)
  }

  function prev() {
    if (!animating.current) snapTo((idxRef.current - 1 + photos.length) % photos.length, true)
  }

  function next() {
    if (!animating.current) snapTo((idxRef.current + 1) % photos.length, true)
  }

  return (
    <div ref={layerRef} className="fixed inset-0 z-500 flex flex-col bg-black/92 select-none">
      <button onClick={close} aria-label="关闭图片预览" className="absolute top-4 right-4 z-10 flex h-9 w-9 items-center justify-center rounded-full bg-white/15 text-lg text-white">x</button>
      {photos.length > 1 && (
        <>
          <button onClick={prev} className="absolute left-3 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-lg text-white active:bg-white/25">←</button>
          <button onClick={next} className="absolute right-3 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-lg text-white active:bg-white/25">→</button>
        </>
      )}
      <div
        className="flex-1 overflow-hidden"
        onTouchStart={onTouchStart}
        onTouchMove={onTouchMove}
        onTouchEnd={onTouchEnd}
        style={{ touchAction: "pan-y" }}
      >
        <div
          ref={stripRef}
          className="flex h-full will-change-transform"
          style={{ width: `${photos.length * 100}vw` }}
        >
          {photos.map((src, i) => (
            <div key={i} className="flex shrink-0 items-center justify-center" style={{ width: "100vw" }}>
              <img src={src} alt="" className="max-h-[75vh] max-w-[90vw] rounded-lg object-contain" draggable={false} />
            </div>
          ))}
        </div>
      </div>
      <div className="pb-6 text-center text-[13px] text-white/50">{idx + 1} / {photos.length}</div>
    </div>
  )
}
