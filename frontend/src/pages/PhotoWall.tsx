import RequestState from "@/components/RequestState"
import { useState, useMemo } from "react"
import { createPortal } from "react-dom"
import { useNavigate } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { photoWallApi } from "@/api"
import PhotoViewer from "@/components/PhotoViewer"
import PageHeader from "@/components/PageHeader"
import type { PhotoWallDay } from "@/types"
import { Images } from "lucide-react"

const moodEmojiMap: Record<string, string> = { yum: "😋", ok: "😐", no: "😵", great: "😋", meh: "😕" }
const moodLabelMap: Record<string, string> = { yum: "好吃", ok: "一般", no: "不想再吃", great: "超满足", meh: "还行吧" }
const dayMoodLabelMap: Record<string, string> = { great: "超满足的一天", ok: "还行的一天", meh: "不太行的一天" }
const homeMoodEmojiMap: Record<string, string> = { happy: "😊", tired: "😫", lazy: "😌", spicy: "🤤", healthy: "🌿" }
const homeMoodLabelMap: Record<string, string> = { happy: "开心", tired: "疲惫", lazy: "想偷懒", spicy: "想吃辣", healthy: "想养生" }

const weekdays = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]

function formatDate(dateStr: string): { month: string; day: string; year: string; weekday: string } {
  const [y, m, d] = dateStr.split("-").map(Number)
  const wd = new Date(y, m - 1, d).getDay()
  return {
    year: String(y),
    month: String(m).padStart(2, "0"),
    day: String(d).padStart(2, "0"),
    weekday: weekdays[wd] || "",
  }
}

// 给每张照片一个轻微、稳定的倾斜角度，营造手贴照片的回忆感
function tiltFor(seed: number): number {
  const angles = [-2.5, 1.8, -1.2, 2.2, -2, 1.4, -1.6, 2.6]
  return angles[seed % angles.length]
}

function MemoryDay({ day, onOpen }: { day: PhotoWallDay; onOpen: (photos: string[], idx: number) => void }) {
  const navigate = useNavigate()
  const f = formatDate(day.meal_date)
  const photos = day.photos || []
  const records = day.records || []
  const cover = photos.slice(0, 4)
  const extra = photos.length - cover.length

  return (
    <div className="relative pl-12">
      {/* 时间线圆点 */}
      <div className="absolute left-3 top-1.5 w-3 h-3 rounded-full bg-primary ring-4 ring-primary-light z-10" />

      {/* 日期标题 */}
      <div className="flex items-baseline gap-2 mb-3">
        <span className="text-xl font-extrabold text-text tracking-tight">{f.month}.{f.day}</span>
        <span className="text-[11px] text-text3 font-medium">{f.year} · {f.weekday}</span>
        {(day.home_mood || day.day_mood) && (
          <div className="ml-auto flex items-center gap-1.5 flex-wrap justify-end">
            {day.home_mood && (
              <span className="text-[11px] font-medium text-primary bg-primary-light px-2 py-0.5 rounded-full border border-primary/10 flex items-center gap-1">
                <span className="text-sm">{homeMoodEmojiMap[day.home_mood] || "😃"}</span>
                {homeMoodLabelMap[day.home_mood] || "心情"}
              </span>
            )}
            {day.day_mood && (
              <span className="text-[11px] font-medium text-text2 bg-card px-2 py-0.5 rounded-full border border-border flex items-center gap-1">
                <span className="text-sm">{moodEmojiMap[day.day_mood] || "🍚"}</span>
                {dayMoodLabelMap[day.day_mood] || "记录"}
              </span>
            )}
          </div>
        )}
      </div>

      {/* 照片拼贴卡片 */}
      <div className="bg-card rounded-2xl p-3 shadow-[0_1px_3px_rgba(0,0,0,.04),0_6px_20px_rgba(0,0,0,.06)] border border-border mb-7">
        <div className={`grid gap-2 ${cover.length === 1 ? "grid-cols-1" : "grid-cols-2"}`}>
          {cover.map((p, i) => (
            <div
              key={i}
              onClick={() => onOpen(photos, i)}
              className="relative cursor-pointer active:scale-[.97] transition-transform"
              style={{ transform: `rotate(${tiltFor(day.meal_date.length + i)}deg)` }}
            >
              <div className="bg-white p-1.5 pb-5 rounded-[6px] shadow-[0_2px_8px_rgba(0,0,0,.10)] border border-[#f0eee9]">
                <div className={`overflow-hidden bg-bg rounded-[3px] ${cover.length === 1 ? "aspect-[4/3]" : "aspect-square"}`}>
                  <img src={p} alt="" className="w-full h-full object-cover" loading="lazy" />
                </div>
              </div>
              {i === cover.length - 1 && extra > 0 && (
                <div className="absolute inset-1.5 bottom-5 rounded-[3px] bg-black/45 flex items-center justify-center text-white text-base font-semibold pointer-events-none">
                  +{extra}
                </div>
              )}
            </div>
          ))}
        </div>

        {/* 当日整体备注 */}
        {day.day_remark && (
          <div className="mt-2.5 px-1 text-[13px] text-text2 leading-relaxed italic">“{day.day_remark}”</div>
        )}

        {/* 当日菜品评价 */}
        {records.length > 0 && (
          <div className="mt-3 pt-3 border-t border-dashed border-border flex flex-col gap-1.5">
            {records.map((r, i) => (
              <div
                key={i}
                onClick={() => navigate(`/dishes/${r.dish_id}`)}
                className="flex items-center gap-2 cursor-pointer active:opacity-70 transition-opacity"
              >
                <span className={`w-[6px] h-[6px] rounded-full flex-shrink-0 ${r.meal_type === "lunch" ? "bg-primary" : "bg-mint"}`} />
                <span className="text-[12px] text-text3 w-7 flex-shrink-0">{r.meal_type === "lunch" ? "午餐" : "晚餐"}</span>
                <span className="text-[13px] font-medium text-text truncate">{r.dish_name}</span>
                {r.mood && (
                  <span className="ml-auto flex items-center gap-1 flex-shrink-0">
                    <span className="text-sm">{moodEmojiMap[r.mood]}</span>
                    <span className="text-[11px] text-text3">{moodLabelMap[r.mood]}</span>
                  </span>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

export default function PhotoWall() {
  const navigate = useNavigate()
  const { data, isLoading , error: loadError, refetch: retryLoad } = useQuery({ queryKey: ["photo-wall"], queryFn: () => photoWallApi.get() })

  const [viewerPhotos, setViewerPhotos] = useState<string[]>([])
  const [viewerIdx, setViewerIdx] = useState(0)
  const [showViewer, setShowViewer] = useState(false)

  function openViewer(photos: string[], idx: number) {
    setViewerPhotos(photos)
    setViewerIdx(idx)
    setShowViewer(true)
  }

  const days = useMemo(() => data?.days || [], [data])

  if (loadError) return <div><PageHeader title="照片墙" onBack={() => navigate(-1)} /><RequestState error={loadError} onRetry={() => { void retryLoad() }} /></div>

  return (
    <div className="animate-fadeUp">
      <PageHeader
        title="照片墙"
        subtitle="把饭桌上的片刻收起来"
        icon={Images}
        meta={data && data.total_photos > 0 ? `${data.total_days} 天 · ${data.total_photos} 张` : undefined}
      />

      <div className="px-5 py-5 max-w-[640px] mx-auto">
        {isLoading ? (
          <div className="flex flex-col gap-6">
            {[0, 1, 2].map((i) => (
              <div key={i} className="pl-12">
                <div className="skeleton h-5 w-28 rounded-md mb-3" />
                <div className="skeleton h-44 w-full rounded-2xl" />
              </div>
            ))}
          </div>
        ) : days.length === 0 ? (
          <div className="text-center py-20">
            <span className="text-[56px] block mb-4 animate-float">📷</span>
            <div className="text-base font-semibold mb-1.5">还没有回忆照片</div>
            <div className="text-[13px] text-text2 mb-6">在「记录」里给每天的饭菜拍张照<br />日子久了，这里就是你们的回忆墙~</div>
            <button
              onClick={() => navigate("/history")}
              className="px-5 py-2.5 rounded-full bg-primary text-white text-sm font-semibold active:scale-95 transition-transform"
            >去记录今天</button>
          </div>
        ) : (
          <>
            <div className="text-center mb-7">
              <div className="text-[13px] text-text2 leading-relaxed">
                🍚 每一餐，都是我们一起走过的日子
              </div>
            </div>
            {/* 时间线 */}
            <div className="relative">
              <div className="absolute left-[18px] top-2 bottom-2 w-[2px] bg-border" />
              <div className="flex flex-col">
                {days.map((day) => (
                  <MemoryDay key={day.meal_date} day={day} onOpen={openViewer} />
                ))}
              </div>
            </div>
            <div className="text-center text-[12px] text-text3 py-6 flex items-center justify-center gap-2">
              <span className="w-8 h-px bg-border2" />
              到这里啦
              <span className="w-8 h-px bg-border2" />
            </div>
          </>
        )}
      </div>

      {showViewer && viewerPhotos.length > 0 && createPortal(
        <PhotoViewer photos={viewerPhotos} idx={viewerIdx} setIdx={setViewerIdx} onClose={() => setShowViewer(false)} />,
        document.body
      )}
    </div>
  )
}
