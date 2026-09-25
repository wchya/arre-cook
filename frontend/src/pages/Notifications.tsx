import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Bell, ChevronRight, ShieldCheck, Sparkles, Wrench, HeartPulse, UsersRound } from "lucide-react"
import { useNavigate } from "react-router-dom"
import toast from "react-hot-toast"
import PageHeader from "@/components/PageHeader"
import { errorMessage, notificationsApi } from "@/api"
import type { Notification } from "@/types"

const meta: Record<string, { icon: typeof Bell; tone: string; label: string }> = {
  system_update: { icon: Bell, tone: "bg-primary-light text-primary", label: "系统" },
  feature: { icon: Sparkles, tone: "bg-purple-light text-purple", label: "新功能" },
  maintenance: { icon: Wrench, tone: "bg-yellow-light text-yellow-dark", label: "维护" },
  health_tip: { icon: HeartPulse, tone: "bg-mint-light text-mint", label: "健康" },
  agent_security: { icon: ShieldCheck, tone: "bg-red-light text-red", label: "安全" },
  family: { icon: UsersRound, tone: "bg-yellow-light text-yellow-dark", label: "家庭" },
}

function timeLabel(raw: string) {
  const date = new Date(raw)
  const diff = Date.now() - date.getTime()
  if (diff < 60_000) return "刚刚"
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  return date.toLocaleDateString("zh-CN", { month: "short", day: "numeric" })
}

export default function Notifications() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading, isError } = useQuery({ queryKey: ["notifications"], queryFn: () => notificationsApi.list({ pageSize: 50 }) })
  const readMut = useMutation({
    mutationFn: (id: number) => notificationsApi.markRead(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["notifications"] }),
    onError: (err) => toast.error(errorMessage(err)),
  })
  const allMut = useMutation({
    mutationFn: notificationsApi.markAllRead,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["notifications"] }),
    onError: (err) => toast.error(errorMessage(err)),
  })
  const items = data?.items || []
  return (
    <div className="min-h-dvh bg-bg pb-8">
      <PageHeader title="站内信" subtitle={data?.unread ? `${data.unread} 条未读消息` : "消息与服务提醒"} icon={Bell} onBack={() => navigate(-1)} actions={data?.unread ? <button onClick={() => allMut.mutate()} disabled={allMut.isPending} className="rounded-md px-2 py-1 text-xs font-semibold text-text2 hover:bg-card">全部已读</button> : undefined} />
      <main className="mx-auto max-w-[640px] space-y-2 px-5 py-4">
        {isLoading ? <div className="h-24 animate-pulse rounded-lg bg-card" /> : isError ? <div className="rounded-lg border border-border bg-card p-5 text-sm text-text2">消息暂时无法加载，请稍后重试。</div> : items.length === 0 ? <div className="rounded-lg border border-dashed border-border bg-card px-5 py-12 text-center"><Bell size={28} className="mx-auto text-text3" /><p className="mt-3 text-sm font-semibold">暂无站内信</p><p className="mt-1 text-xs text-text3">系统更新、健康提醒和安全事件会显示在这里</p></div> : items.map((item) => <NotificationRow key={item.id} item={item} onRead={() => !item.read_at && readMut.mutate(item.id)} onOpen={() => { if (item.link) navigate(item.link) }} />)}
      </main>
    </div>
  )
}

function NotificationRow({ item, onRead, onOpen }: { item: Notification; onRead: () => void; onOpen: () => void }) {
  const info = meta[item.type] || meta.system_update
  const Icon = info.icon
  return <button onClick={() => { onRead(); onOpen() }} className={`flex w-full items-start gap-3 rounded-lg border p-4 text-left transition-colors hover:bg-card ${item.read_at ? "border-border bg-card" : "border-primary/25 bg-primary-light/30"}`}>
    <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-[12px] ${info.tone}`}><Icon size={17} /></span>
    <span className="min-w-0 flex-1"><span className="flex items-center gap-2"><span className={`truncate text-sm font-bold ${item.read_at ? "text-text" : "text-text"}`}>{item.title}</span>{!item.read_at && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />}</span><span className="mt-1 block whitespace-pre-wrap text-[13px] leading-6 text-text2">{item.content}</span><span className="mt-2 block text-[11px] text-text3">{info.label} · {timeLabel(item.created_at)}</span></span>{item.link && <ChevronRight size={16} className="mt-2 shrink-0 text-text4" />}</button>
}
