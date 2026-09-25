import { useRef, useState, type ReactNode } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { achievementsApi, errorMessage, getUploadErrorMessage, meApi, notificationsApi, settingsApi, statsApi, suggestionsApi, uploadApi } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import { asString } from "@/lib/utils"
import AnimatedBottomSheet from "@/components/AnimatedBottomSheet"
import toast from "react-hot-toast"
import {
  Bell, Bot, Camera, ChevronRight, Heart, Images, Inbox, LayoutDashboard, LogOut, NotebookPen, Plug, Salad, Shield,
  ShoppingBasket, Sparkles, Trophy, UserRound, UsersRound, Activity, type LucideIcon,
} from "lucide-react"

function Row({ icon: Icon, tone, title, desc, badge, onClick, right }: {
  icon: LucideIcon
  tone: string
  title: string
  desc?: string
  badge?: number
  onClick?: () => void
  right?: ReactNode
}) {
  return (
    <button onClick={onClick} className="flex w-full items-center gap-3 px-4 py-3.5 text-left transition-colors active:bg-bg">
      <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-[12px] ${tone}`}>
        <Icon size={18} strokeWidth={2.3} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[14px] font-semibold text-text">{title}</span>
        {desc && <span className="block truncate text-[11px] text-text3">{desc}</span>}
      </span>
      {badge ? <span className="rounded-full bg-red px-1.5 text-[11px] font-bold leading-[18px] text-white">{badge}</span> : null}
      {right ?? <ChevronRight size={16} className="shrink-0 text-text4" />}
    </button>
  )
}

function Group({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section className="mb-4">
      {title && <div className="mb-2 px-1 text-[12px] font-bold tracking-wide text-text3">{title}</div>}
      <div className="divide-y divide-border overflow-hidden rounded-[22px] border border-border bg-card shadow-[0_1px_3px_rgba(0,0,0,.03),0_8px_24px_rgba(0,0,0,.04)]">{children}</div>
    </section>
  )
}

function Switch({ on }: { on: boolean }) {
  return (
    <span className={`relative h-6 w-11 shrink-0 rounded-full transition-colors ${on ? "bg-primary" : "bg-border2"}`}>
      <span className={`absolute top-[2px] h-5 w-5 rounded-full bg-white shadow-sm transition-all ${on ? "left-[22px]" : "left-[2px]"}`} />
    </span>
  )
}

export default function Me() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const logout = useAuthStore((s) => s.logout)
  const appName = useAppInfoStore((s) => s.appName)
  const fileRef = useRef<HTMLInputElement>(null)
  const [editingName, setEditingName] = useState(false)
  const [nickname, setNickname] = useState("")
  const [repeatSheet, setRepeatSheet] = useState(false)

  const { data: stats } = useQuery({ queryKey: ["stats"], queryFn: () => statsApi.get() })
  const { data: pending = [] } = useQuery({ queryKey: ["suggestions", "pending"], queryFn: () => suggestionsApi.list("pending") })
  const { data: notifications } = useQuery({ queryKey: ["notifications"], queryFn: () => notificationsApi.list({ pageSize: 1 }) })
  const { data: achievements = [] } = useQuery({ queryKey: ["achievements"], queryFn: () => achievementsApi.list() })
  const { data: settings } = useQuery({ queryKey: ["settings"], queryFn: () => settingsApi.get() })

  const settingsMut = useMutation({
    mutationFn: (s: Record<string, string>) => settingsApi.update(s),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings"] })
      qc.invalidateQueries({ queryKey: ["week-plan"] })
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const profileMut = useMutation({
    mutationFn: (data: { nickname?: string; avatar?: string }) => meApi.update(data),
    onSuccess: (u) => {
      setUser(u)
      toast.success("已更新")
      setEditingName(false)
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  async function onAvatar(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (!file) return
    try {
      const res = await uploadApi.image(file)
      profileMut.mutate({ avatar: res.data.data.url })
    } catch (err) {
      toast.error(getUploadErrorMessage(err))
    }
  }

  if (!user) return null
  const unlocked = achievements.filter((a) => a.is_unlocked).length
  const voiceOn = asString(settings?.voice_enabled, "1") !== "0"
  const blindOn = asString(settings?.blind_box_enabled, "1") !== "0"
  const repeatDays = asString(settings?.repeat_days, "3")

  const statTiles = [
    { label: "记录", value: stats?.total_records ?? "-" },
    { label: "连续天数", value: stats?.current_streak ?? "-" },
    { label: "吃过的菜", value: stats?.distinct_dishes ?? "-" },
    { label: "收藏", value: stats?.favorite_count ?? "-" },
  ]

  return (
    <div className="animate-fadeUp">
      <div className="relative overflow-hidden bg-gradient-to-br from-[#FFE9DE] via-primary-light to-bg px-5 pb-6" style={{ paddingTop: "calc(20px + env(safe-area-inset-top))" }}>
        <div className="pointer-events-none absolute -right-10 -top-10 h-44 w-44 rounded-full bg-primary/15 blur-2xl" />
        <div className="relative mx-auto flex max-w-[640px] items-center gap-4">
          <button onClick={() => fileRef.current?.click()} className="relative shrink-0" aria-label="更换头像">
            {user.avatar ? (
              <img src={user.avatar} alt="" className="h-[68px] w-[68px] rounded-[24px] object-cover shadow-[0_10px_24px_rgba(232,115,74,.25)] ring-4 ring-white/70" />
            ) : (
              <span className="flex h-[68px] w-[68px] items-center justify-center rounded-[24px] bg-gradient-to-br from-[#F59A6B] to-primary text-[28px] font-black text-white shadow-[0_10px_24px_rgba(232,115,74,.3)] ring-4 ring-white/70">
                {(user.nickname || "我").slice(0, 1).toUpperCase()}
              </span>
            )}
            <span className="absolute -bottom-1 -right-1 flex h-6 w-6 items-center justify-center rounded-full bg-card text-text2 shadow">
              <Camera size={13} strokeWidth={2.4} />
            </span>
          </button>
          <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={onAvatar} />
          <div className="min-w-0 flex-1">
            <button onClick={() => { setNickname(user.nickname); setEditingName(true) }} className="flex max-w-full items-center gap-1.5 text-left">
              <span className="truncate text-[21px] font-black tracking-tight text-text">{user.nickname}</span>
              <NotebookPen size={14} className="shrink-0 text-text3" />
            </button>
            <div className="mt-0.5 truncate text-[12px] text-text2">{user.email || user.username}</div>
            <div className="mt-1.5 flex gap-1.5">
              {user.role === "admin" && <span className="rounded-full bg-text px-2 py-0.5 text-[10px] font-bold text-bg">管理员</span>}
              {user.wechat_bound && <span className="rounded-full bg-mint-light px-2 py-0.5 text-[10px] font-bold text-mint">已绑定微信</span>}
              <span className="rounded-full bg-card/80 px-2 py-0.5 text-[10px] font-bold text-text2">🏆 {unlocked} 个成就</span>
            </div>
          </div>
        </div>

        <div className="relative mx-auto mt-5 grid max-w-[640px] grid-cols-4 gap-2">
          {statTiles.map((s) => (
            <div key={s.label} className="rounded-2xl bg-card/80 px-2 py-2.5 text-center shadow-[0_1px_2px_rgba(0,0,0,.03)] backdrop-blur">
              <div className="text-[19px] font-black leading-tight text-text">{s.value}</div>
              <div className="text-[10px] font-semibold text-text3">{s.label}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="mx-auto max-w-[640px] px-5 pt-4">
        <Group title="AI 与口味">
          <Row icon={Bell} tone="bg-primary-light text-primary" title="站内信" desc={notifications?.unread ? `${notifications.unread} 条未读消息` : "系统更新、健康与安全提醒"} badge={notifications?.unread} onClick={() => navigate("/notifications")} />
          <Row icon={Inbox} tone="bg-primary-light text-primary" title="AI 建议" desc={pending.length > 0 ? `${pending.length} 条待处理` : "智能体推送给你的菜单建议"} badge={pending.length} onClick={() => navigate("/suggestions")} />
          <Row icon={Sparkles} tone="bg-purple-light text-purple" title="口味画像" desc="看看 AI 眼中的你，按条件挑菜" onClick={() => navigate("/taste-profile")} />
          <Row icon={Salad} tone="bg-mint-light text-mint" title="饮食偏好" desc="忌口、过敏、辣度、做饭时长" onClick={() => navigate("/me/preferences")} />
          <Row icon={Plug} tone="bg-yellow-light text-yellow-dark" title="AI 连接" desc="接入 DeepSeek、Hermes 等第三方智能体" onClick={() => navigate("/me/ai")} />
        </Group>

        <Group title="我的内容">
          <Row icon={Activity} tone="bg-mint-light text-mint" title="饮食记录与报告" desc="记录实际吃的菜系，查看饮食变化" onClick={() => navigate("/health")} />
          <Row icon={UsersRound} tone="bg-yellow-light text-yellow-dark" title="我的家庭" desc="共享菜谱、家庭菜单与买菜清单" onClick={() => navigate("/family")} />
          <Row icon={Heart} tone="bg-pink-light text-pink" title="我的收藏" onClick={() => navigate("/favorites")} />
          <Row icon={ShoppingBasket} tone="bg-mint-light text-mint" title="一周菜单与买菜清单" onClick={() => navigate("/plan")} />
          <Row icon={Images} tone="bg-primary-light text-primary" title="照片墙" onClick={() => navigate("/photo-wall")} />
          <Row icon={Trophy} tone="bg-yellow-light text-yellow-dark" title="成就" desc={`已解锁 ${unlocked}/${achievements.length}`} onClick={() => navigate("/achievements")} />
          <Row icon={Bot} tone="bg-purple-light text-purple" title="新建私房菜" desc={stats?.private_dishes ? `已有 ${stats.private_dishes} 道私房菜` : "只有你自己看得到"} onClick={() => navigate("/dishes/new")} />
        </Group>

        <Group title="偏好设置">
          <Row icon={Sparkles} tone="bg-bg text-text2" title="推荐去重" desc={`近 ${repeatDays} 天吃过的菜不优先推荐`} onClick={() => setRepeatSheet(true)} right={<span className="text-[13px] font-semibold text-text3">{repeatDays} 天 ›</span>} />
          <Row icon={Bot} tone="bg-bg text-text2" title="语音播报" desc="做菜时朗读步骤" onClick={() => settingsMut.mutate({ voice_enabled: voiceOn ? "0" : "1" })} right={<Switch on={voiceOn} />} />
          <Row icon={Sparkles} tone="bg-bg text-text2" title="惊喜盲盒" desc="首页显示盲盒" onClick={() => settingsMut.mutate({ blind_box_enabled: blindOn ? "0" : "1" })} right={<Switch on={blindOn} />} />
        </Group>

        <Group>
          <Row icon={Shield} tone="bg-bg text-text2" title="账号与安全" desc="密码、登录设备、导出与注销" onClick={() => navigate("/me/account")} />
          {user.role === "admin" && <Row icon={LayoutDashboard} tone="bg-text text-bg" title="管理后台" desc="菜谱、用户、AI 模型与站点设置" onClick={() => navigate("/admin/dashboard")} />}
        </Group>

        <button
          onClick={() => {
            if (confirm("确定退出登录吗？")) void logout()
          }}
          className="mb-6 flex h-12 w-full items-center justify-center gap-2 rounded-[18px] border border-border bg-card text-[14px] font-bold text-red transition-all active:scale-[.98]"
        >
          <LogOut size={16} strokeWidth={2.4} /> 退出登录
        </button>
        <div className="pb-4 text-center text-[11px] text-text4">
          <UserRound size={11} className="mr-1 inline" />
          {appName} · 你的数据只属于你
        </div>
      </div>

      {editingName && (
        <AnimatedBottomSheet onClose={() => setEditingName(false)} className="rounded-t-3xl p-5 pb-8">
          {({ close }) => (
            <>
              <div className="mx-auto mb-4 h-1 w-10 rounded-full bg-border2" />
              <div className="mb-3 text-[17px] font-extrabold">修改昵称</div>
              <input
                autoFocus
                value={nickname}
                maxLength={20}
                onChange={(e) => setNickname(e.target.value)}
                className="mb-4 h-12 w-full rounded-2xl border-[1.5px] border-border bg-bg px-4 text-[15px] outline-none focus:border-primary"
              />
              <div className="flex gap-3">
                <button onClick={close} className="h-11 flex-1 rounded-full border-[1.5px] border-border text-sm font-semibold">取消</button>
                <button
                  onClick={() => nickname.trim() && profileMut.mutate({ nickname: nickname.trim() })}
                  disabled={!nickname.trim() || profileMut.isPending}
                  className="h-11 flex-1 rounded-full bg-primary text-sm font-bold text-white disabled:opacity-50"
                >
                  保存
                </button>
              </div>
            </>
          )}
        </AnimatedBottomSheet>
      )}

      {repeatSheet && (
        <AnimatedBottomSheet onClose={() => setRepeatSheet(false)} className="rounded-t-3xl p-5 pb-8">
          {({ close }) => (
            <>
              <div className="mx-auto mb-4 h-1 w-10 rounded-full bg-border2" />
              <div className="mb-1 text-[17px] font-extrabold">推荐去重天数</div>
              <div className="mb-4 text-[12px] text-text3">这几天内吃过的菜，推荐时会先避开</div>
              <div className="grid grid-cols-4 gap-2">
                {["1", "2", "3", "5", "7", "10", "14", "0"].map((d) => (
                  <button
                    key={d}
                    onClick={() => {
                      settingsMut.mutate({ repeat_days: d === "0" ? "" : d })
                      close()
                    }}
                    className={`h-11 rounded-2xl border-[1.5px] text-[13px] font-bold ${repeatDays === d ? "border-primary bg-primary-light text-primary" : "border-border bg-bg text-text2"}`}
                  >
                    {d === "0" ? "默认" : `${d} 天`}
                  </button>
                ))}
              </div>
            </>
          )}
        </AnimatedBottomSheet>
      )}
    </div>
  )
}
