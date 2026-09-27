import RequestState from "@/components/RequestState"
import { useNavigate } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { achievementsApi } from "@/api"
import PageHeader from "@/components/PageHeader"
import type { Achievement } from "@/types"

function formatUnlockedAt(value?: string | null) {
  if (!value) return ""
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ""
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

export default function Achievements() {
  const navigate = useNavigate()

  const { data: rawAchievements , error: loadError, refetch: retryLoad, isLoading: pageLoading } = useQuery({ queryKey: ["achievements"], queryFn: () => achievementsApi.list() })
  const achievements = Array.isArray(rawAchievements) ? rawAchievements : []

  const unlockedCount = achievements.filter((a: Achievement) => a.is_unlocked).length
  const autoCount = achievements.filter((a: Achievement) => a.condition === "auto").length
  const manualCount = achievements.length - autoCount
  const progress = achievements.length > 0 ? Math.round((unlockedCount / achievements.length) * 100) : 0
  const sortedAchievements = achievements.slice().sort((a: Achievement, b: Achievement) => {
    if (a.is_unlocked !== b.is_unlocked) return a.is_unlocked ? -1 : 1
    return a.id - b.id
  })

  if (loadError) return <div><PageHeader title="成就墙" onBack={() => navigate(-1)} /><RequestState error={loadError} onRetry={() => { void retryLoad() }} /></div>

  if (pageLoading) return <div><PageHeader title="成就墙" onBack={() => navigate(-1)} /><RequestState /></div>

  return (
    <div className="animate-fadeUp">
      <PageHeader
        title="成就墙"
        subtitle="记录你们一起解锁的小目标"
        meta={`${unlockedCount}/${achievements.length}`}
        onBack={() => navigate(-1)}
      />

      <div className="px-5 py-4 max-w-[640px] mx-auto">
        <div className="bg-card rounded-2xl border border-border p-4 mb-4 shadow-[0_1px_3px_rgba(0,0,0,.04),0_4px_12px_rgba(0,0,0,.04)]">
          <div className="flex items-end justify-between mb-2.5">
            <div>
              <div className="text-[13px] text-text2 mb-0.5">成就进度</div>
              <div className="text-2xl font-extrabold tracking-tight">{progress}%</div>
            </div>
            <div className="text-right text-xs text-text2 leading-relaxed">
              <div>自动 {autoCount}</div>
              <div>手动 {manualCount}</div>
            </div>
          </div>
          <div className="h-2 rounded-full bg-bg overflow-hidden">
            <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${progress}%` }} />
          </div>
        </div>

        <div className="grid grid-cols-3 gap-3">
          {sortedAchievements.map((a: Achievement) => (
            <div
              key={a.id}
              className={`bg-card rounded-2xl p-3.5 pt-4 text-center shadow-[0_1px_3px_rgba(0,0,0,.04),0_4px_12px_rgba(0,0,0,.04)] border transition-all relative overflow-hidden ${a.is_unlocked ? "border-primary/20" : "border-border opacity-45 grayscale-[.9]"}`}
            >
              {a.is_unlocked && <div className="absolute top-0 left-0 right-0 h-[3px] bg-primary" />}
              <span className="text-[32px] block mb-1.5">{a.icon}</span>
              <div className="text-xs font-semibold mb-0.5">{a.name}</div>
              <div className="text-[10px] text-text2 leading-snug min-h-[28px]">{a.description}</div>
              <div className={`mt-2 inline-flex items-center justify-center px-2 py-0.5 rounded-full text-[9px] font-semibold ${a.is_unlocked ? "bg-primary-light text-primary" : "bg-bg text-text3"}`}>
                {a.is_unlocked ? `已解锁 ${formatUnlockedAt(a.unlocked_at)}` : a.condition === "auto" ? "自动检测" : "手动成就"}
              </div>
            </div>
          ))}
        </div>
        {achievements.length === 0 && (
          <div className="py-16 text-center text-sm text-text3">还没有成就</div>
        )}
      </div>
    </div>
  )
}
