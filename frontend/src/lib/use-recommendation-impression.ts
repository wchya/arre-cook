import { useEffect, useRef, type RefObject } from "react"
import { recommendationFeedbackApi } from "@/api"

// Preloaded candidates are not exposures. Record only when the card is visible.
export function useRecommendationImpression(ref: RefObject<HTMLElement | null>, recommendationId: string, dishId?: number) {
  const sent = useRef(new Set<string>())
  useEffect(() => {
    const element = ref.current
    if (!element || !recommendationId || !dishId || typeof IntersectionObserver === "undefined") return
    const key = `${recommendationId}:${dishId}`
    if (sent.current.has(key)) return
    let visible = false
    const report = () => {
      if (!visible || document.visibilityState === "hidden" || sent.current.has(key)) return
      sent.current.add(key)
      void recommendationFeedbackApi.send(recommendationId, dishId, "impression")
        .then(() => observer.disconnect())
        .catch(() => sent.current.delete(key))
    }
    const observer = new IntersectionObserver((entries) => {
      visible = entries.some((entry) => entry.isIntersecting)
      report()
    }, { threshold: 0.5 })
    observer.observe(element)
    document.addEventListener("visibilitychange", report)
    return () => { observer.disconnect(); document.removeEventListener("visibilitychange", report) }
  }, [ref, recommendationId, dishId])
}
