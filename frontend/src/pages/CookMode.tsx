import PageHeader from "@/components/PageHeader"
import RequestState from "@/components/RequestState"
import { useState, useRef, useMemo, useCallback } from "react"
import { useParams, useNavigate } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { dishesApi } from "@/api"
import { asArray } from "@/lib/utils"
import toast from "react-hot-toast"

interface Step { text: string; time?: number }

function parseSteps(value: unknown): Step[] {
  return asArray(value).map((item) => {
    if (typeof item === "string") return { text: item }
    if (typeof item === "object" && item !== null) return { text: (item as Record<string, unknown>).text as string || "", time: (item as Record<string, unknown>).time as number | undefined }
    return { text: String(item) }
  })
}

export default function CookMode() {
  const { id } = useParams()
  const navigate = useNavigate()
  const dishId = Number(id)

  const { data: dish , error: loadError, refetch: retryLoad } = useQuery({
    queryKey: ["dish", dishId],
    queryFn: () => dishesApi.get(dishId),
    enabled: !!dishId,
  })

  const steps = useMemo(() => dish ? parseSteps(dish.steps) : [], [dish])
  const [currentStep, setCurrentStep] = useState(0)
  const [secondsLeft, setSecondsLeft] = useState(0)
  const [activeTotal, setActiveTotal] = useState(0)
  const [paused, setPaused] = useState(true)
  const [timerActive, setTimerActive] = useState(false)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const stepTime = steps[currentStep]?.time || 0
  const totalSec = stepTime * 60

  const speakStep = useCallback(() => {
    if ("speechSynthesis" in window && steps[currentStep]) {
      speechSynthesis.cancel()
      const u = new SpeechSynthesisUtterance(steps[currentStep].text)
      u.lang = "zh-CN"; u.rate = 0.9
      speechSynthesis.speak(u)
      toast("🔊 正在播报当前步骤")
    }
  }, [steps, currentStep])

  function startTimer(secs: number) {
    if (timerRef.current) clearInterval(timerRef.current)
    setActiveTotal(secs)
    setSecondsLeft(secs)
    setPaused(false)
    setTimerActive(true)
    timerRef.current = setInterval(() => {
      setSecondsLeft((s) => {
        if (s <= 1) {
          clearInterval(timerRef.current!)
          timerRef.current = null
          setPaused(true)
          setTimerActive(false)
          toast("⏰ 时间到！该进行下一步了~", { icon: "⏰" })
          speakStep()
          setTimeout(() => {
            setCurrentStep((prev) => {
              if (prev < steps.length - 1) return prev + 1
              toast.success("🎉 全部步骤完成！")
              return prev
            })
          }, 3000)
          return 0
        }
        return s - 1
      })
    }, 1000)
  }

  function toggleTimer() {
    if (paused) {
      const secs = secondsLeft > 0 ? secondsLeft : totalSec
      if (secs > 0) startTimer(secs)
    } else {
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null }
      setPaused(true)
      setTimerActive(false)
    }
  }

  function goToStep(idx: number) {
    if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null }
    setPaused(true)
    setTimerActive(false)
    setSecondsLeft(0)
    setCurrentStep(idx)
  }

  if (loadError) return <div><PageHeader title="烹饪步骤" onBack={() => navigate(-1)} /><RequestState error={loadError} onRetry={() => { void retryLoad() }} /></div>

  if (!Number.isSafeInteger(dishId) || dishId <= 0) return <div><PageHeader title="菜谱链接无效" onBack={() => navigate(-1)} /><p className="p-6 text-text2">请返回菜谱列表后重新打开。</p></div>
  if (!dish) return <div><PageHeader title="烹饪步骤" onBack={() => navigate(-1)} /><RequestState /></div>

  const step = steps[currentStep]
  const progress = steps.length > 0 ? ((currentStep + 1) / steps.length) * 100 : 0
  const circumference = 2 * Math.PI * 80
  const currentTotal = activeTotal || totalSec
  const offset = currentTotal > 0 ? circumference * (1 - secondsLeft / currentTotal) : 0

  return (
    <div className="fixed inset-0 z-[300] bg-[#1A1A2E] text-white flex flex-col">
      <div className="flex items-center justify-between px-5 py-4">
        <div className="text-lg font-bold">🍳 {dish.name}</div>
        <button onClick={() => { if (timerRef.current) clearInterval(timerRef.current); navigate(-1) }} className="w-9 h-9 rounded-full bg-white/10 flex items-center justify-center text-lg">✕</button>
      </div>

      <div className="px-5 mb-2">
        <div className="h-1 bg-white/10 rounded overflow-hidden">
          <div className="h-full bg-primary rounded transition-all duration-300" style={{ width: `${progress}%` }} />
        </div>
      </div>

      <div className="flex-1 flex flex-col items-center justify-center px-10 text-center">
        <div className="text-sm text-white/50 mb-2">步骤 <span>{currentStep + 1}</span> / <span>{steps.length}</span></div>
        <div className="text-[64px] font-extrabold text-primary leading-none mb-4">{currentStep + 1}</div>
        <div className="text-xl font-semibold leading-relaxed mb-6 max-w-[400px]">{step?.text}</div>

        {step?.time && step.time > 0 && (
          <div className="relative w-[160px] h-[160px] mb-6">
            <div className="w-[160px] h-[160px] rounded-full border-4 border-white/10 relative flex items-center justify-center">
              <svg viewBox="0 0 168 168" className="absolute inset-[-4px]" style={{ transform: "rotate(-90deg)" }}>
                <circle cx="84" cy="84" r="80" fill="none" stroke="#E8734A" strokeWidth="4" strokeLinecap="round"
                  strokeDasharray={circumference} strokeDashoffset={timerActive ? offset : 0} style={{ transition: "stroke-dashoffset .5s ease" }} />
              </svg>
              <div>
                <div className={`text-4xl font-extrabold tracking-wider ${timerActive && secondsLeft <= 0 ? "animate-countdown-pulse" : ""}`}>
                  {!timerActive && secondsLeft === 0 ? `${step.time}:00` : `${Math.floor(secondsLeft / 60)}:${String(secondsLeft % 60).padStart(2, "0")}`}
                </div>
                <div className="text-xs text-white/40 mt-1">{!timerActive ? "准备开始" : paused ? "已暂停" : "进行中..."}</div>
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="flex items-center justify-center gap-5 px-5 py-5" style={{ paddingBottom: "calc(20px + env(safe-area-inset-bottom))" }}>
        <button onClick={() => currentStep > 0 && goToStep(currentStep - 1)} className={`w-14 h-14 rounded-full flex items-center justify-center text-2xl bg-white/10 hover:bg-white/20 transition-all ${currentStep === 0 ? "opacity-30" : ""}`}>◀◀</button>
        <button onClick={speakStep} className="w-14 h-14 rounded-full flex items-center justify-center text-2xl bg-white/10 hover:bg-white/20 transition-all">🔊</button>
        <button onClick={toggleTimer} className="w-[72px] h-[72px] rounded-full flex items-center justify-center text-[28px] bg-primary transition-all active:scale-90">{paused ? "▶" : "⏸"}</button>
        <button onClick={() => currentStep < steps.length - 1 && goToStep(currentStep + 1)} className={`w-14 h-14 rounded-full flex items-center justify-center text-2xl bg-white/10 hover:bg-white/20 transition-all ${currentStep >= steps.length - 1 ? "opacity-30" : ""}`}>▶▶</button>
        <button onClick={() => { if (currentStep < steps.length - 1) goToStep(currentStep + 1); else toast.success("🎉 全部完成！") }} className="w-14 h-14 rounded-full flex items-center justify-center text-2xl bg-white/10 hover:bg-white/20 transition-all">⏭</button>
      </div>
    </div>
  )
}
