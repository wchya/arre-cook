import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { achievementsApi } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import type { Achievement } from "@/types"

const colors = ["#E8734A", "#6EC6B8", "#F5D76E", "#F4A8A0", "#8B5CF6", "#FFFFFF"]

// 已展示过的成就按用户分开记，同一台设备切换账号互不干扰
function storageKey() {
  return `ninimenu_known_unlocked_achievements:${useAuthStore.getState().user?.id ?? 0}`
}

function loadKnownCodes() {
  try {
    const raw = localStorage.getItem(storageKey())
    const parsed = raw ? JSON.parse(raw) : []
    return new Set(Array.isArray(parsed) ? parsed.filter((x) => typeof x === "string") : [])
  } catch {
    return new Set<string>()
  }
}

function saveKnownCodes(codes: Set<string>) {
  try {
    localStorage.setItem(storageKey(), JSON.stringify(Array.from(codes)))
  } catch {
    /* ignore */
  }
}

function sortUnlocked(a: Achievement, b: Achievement) {
  const at = a.unlocked_at ? new Date(a.unlocked_at).getTime() : 0
  const bt = b.unlocked_at ? new Date(b.unlocked_at).getTime() : 0
  if (at !== bt) return at - bt
  return a.id - b.id
}

function confettiStyle(i: number): React.CSSProperties {
  const left = 8 + ((i * 17) % 84)
  const drift = ((i % 7) - 3) * 18
  const delay = (i % 9) * 70
  const duration = 1500 + (i % 5) * 180
  const size = 6 + (i % 4) * 2
  return {
    left: `${left}%`,
    width: `${size}px`,
    height: `${Math.max(4, size - 2)}px`,
    background: colors[i % colors.length],
    ["--drift" as string]: `${drift}px`,
    ["--fall-delay" as string]: `${delay}ms`,
    ["--fall-duration" as string]: `${duration}ms`,
  }
}

export default function AchievementUnlockOverlay() {
  const initializedRef = useRef(false)
  const knownCodesRef = useRef<Set<string> | null>(null)

  const queueRef = useRef<Achievement[]>([])
  const timerRef = useRef({ leave: 0, next: 0 })
  const displayRef = useRef<Achievement | null>(null)

  const [display, setDisplay] = useState<Achievement | null>(null)
  const [leaving, setLeaving] = useState(false)
  const [queueLen, setQueueLen] = useState(0)

  const showNextRef = useRef<() => void>(() => {})

  const showNext = useCallback(() => {
    if (queueRef.current.length === 0) {
      setDisplay(null)
      displayRef.current = null
      setQueueLen(0)
      return
    }
    const next = queueRef.current[0]
    queueRef.current = queueRef.current.slice(1)
    setDisplay(next)
    displayRef.current = next
    setLeaving(false)
    setQueueLen(queueRef.current.length)

    window.clearTimeout(timerRef.current.leave)
    window.clearTimeout(timerRef.current.next)

    timerRef.current.leave = window.setTimeout(() => {
      setLeaving(true)
    }, 2600)

    timerRef.current.next = window.setTimeout(() => {
      showNextRef.current()
    }, 3200)
  }, [])

  useEffect(() => {
    showNextRef.current = showNext
  }, [showNext])

  const { data: rawAchievements } = useQuery({
    queryKey: ["achievements"],
    queryFn: () => achievementsApi.list(),
    refetchOnWindowFocus: true,
  })
  const achievements = Array.isArray(rawAchievements) ? rawAchievements : []

  const unlocked = useMemo(
    () => achievements.filter((a: Achievement) => a.is_unlocked).sort(sortUnlocked),
    [achievements],
  )

  useEffect(() => {
    if (achievements.length === 0) return

    if (!initializedRef.current) {
      initializedRef.current = true
      const known = loadKnownCodes()
      knownCodesRef.current = known

      if (known.size === 0) {
        unlocked.forEach((a) => known.add(a.code))
        saveKnownCodes(known)
        return
      }
    }

    const known = knownCodesRef.current ?? loadKnownCodes()
    const newlyUnlocked = unlocked.filter((a) => !known.has(a.code))
    if (newlyUnlocked.length === 0) return

    newlyUnlocked.forEach((a) => known.add(a.code))
    knownCodesRef.current = known
    saveKnownCodes(known)

    const existing = new Set([
      ...queueRef.current.map((a) => a.code),
      ...(displayRef.current ? [displayRef.current.code] : []),
    ])
    const toAdd = newlyUnlocked.filter((a) => !existing.has(a.code))
    if (toAdd.length === 0) return

    queueRef.current = [...queueRef.current, ...toAdd]
    setQueueLen(queueRef.current.length)

    if (!displayRef.current) {
      showNext()
    }
  }, [achievements.length, unlocked, showNext])

  useEffect(() => {
    return () => {
      window.clearTimeout(timerRef.current.leave)
      window.clearTimeout(timerRef.current.next)
    }
  }, [])

  if (!display) return null

  return (
    <div className={`achievement-unlock-layer ${leaving ? "is-leaving" : ""}`}>
      <div className="achievement-confetti-field" aria-hidden="true">
        {Array.from({ length: 32 }).map((_, i) => (
          <span key={i} className="achievement-confetti-piece" style={confettiStyle(i)} />
        ))}
      </div>
      <div className="achievement-unlock-card">
        <div className="achievement-glow-ring" aria-hidden="true" />
        <div className="achievement-icon-wrap">
          <span className="achievement-icon">{display.icon || "🏆"}</span>
        </div>
        <div className="achievement-kicker">成就已解锁</div>
        <div className="achievement-title">{display.name}</div>
        <div className="achievement-desc">{display.description}</div>
        {queueLen > 0 && <div className="achievement-chain">连续解锁中 · 还剩 {queueLen} 个</div>}
      </div>
    </div>
  )
}
