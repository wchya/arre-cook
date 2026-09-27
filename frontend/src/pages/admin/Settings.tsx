import RequestState from "@/components/RequestState"
import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { adminApi, adminNotificationsApi, shoppingCategoriesApi, errorMessage } from "@/api"
import { asString } from "@/lib/utils"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import type { ShoppingCategoryOverride } from "@/types"
import toast from "react-hot-toast"

const shoppingCategories = ["蔬菜", "肉类", "配料", "其他"]

function SettingRow({ label, children, stacked = false }: { label: string; children: React.ReactNode; stacked?: boolean }) {
  return (
    <div className={`flex gap-3 py-3 border-b border-border/60 last:border-b-0 ${stacked ? "flex-col" : "flex-wrap items-center justify-between"}`}>
      <div className="text-sm font-medium text-text">{label}</div>
      <div className="flex min-w-0 items-center gap-3">{children}</div>
    </div>
  )
}

export default function AdminSettings() {
  const qc = useQueryClient()
  const { data: settings, isLoading , error: loadError, refetch: retryLoad } = useQuery({ queryKey: ["admin-site-settings"], queryFn: () => adminApi.settings() })

  const [repeatDays, setRepeatDays] = useState<string | null>(null)
  const [lunchPerDay, setLunchPerDay] = useState<string | null>(null)
  const [dinnerPerDay, setDinnerPerDay] = useState<string | null>(null)
  const [appName, setAppName] = useState<string | null>(null)
  const [siteLimit, setSiteLimit] = useState<string | null>(null)
  const [assistantLimit, setAssistantLimit] = useState<string | null>(null)
  const [shoppingItemName, setShoppingItemName] = useState("")
  const [shoppingCategory, setShoppingCategory] = useState("蔬菜")
  const [noticeType, setNoticeType] = useState("system_update")
  const [noticeTitle, setNoticeTitle] = useState("")
  const [noticeContent, setNoticeContent] = useState("")
  const storedAppName = useAppInfoStore((s) => s.appName)
  const updateAppName = useAppInfoStore((s) => s.setAppName)

  const updateMut = useMutation({
    mutationFn: (s: Record<string, string>) => adminApi.updateSettings(s),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ["admin-site-settings"] })
      qc.invalidateQueries({ queryKey: ["settings"] })
      if (variables.app_name !== undefined) {
        updateAppName(variables.app_name || "arre食谱推荐小助手")
      }
      if (variables.assistant_daily_limit !== undefined || variables.assistant_site_daily_limit !== undefined) {
        if (variables.assistant_daily_limit !== undefined) setAssistantLimit(null)
        if (variables.assistant_site_daily_limit !== undefined) setSiteLimit(null)
        void qc.invalidateQueries({ queryKey: ["assistant-status"] })
      }
      toast.success("已保存")
    },
    onError: (error) => toast.error(errorMessage(error, "保存失败")),
  })

  const { data: categoryOverrides = [] } = useQuery({
    queryKey: ["shopping-categories"],
    queryFn: () => shoppingCategoriesApi.list(),
  })

  const saveCategoryMut = useMutation({
    mutationFn: (data: { item_name: string; category: string }) => shoppingCategoriesApi.save(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["shopping-categories"] })
      qc.invalidateQueries({ queryKey: ["shopping-list"] })
      setShoppingItemName("")
      toast.success("分类已保存")
    },
    onError: () => toast.error("分类保存失败"),
  })

  const deleteCategoryMut = useMutation({
    mutationFn: (itemName: string) => shoppingCategoriesApi.delete(itemName),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["shopping-categories"] })
      qc.invalidateQueries({ queryKey: ["shopping-list"] })
      toast.success("已恢复智能识别")
    },
    onError: () => toast.error("删除失败"),
  })

  const noticeMut = useMutation({
    mutationFn: () => adminNotificationsApi.publish({ type: noticeType, title: noticeTitle.trim(), content: noticeContent.trim() }),
    onSuccess: (result) => {
      setNoticeTitle("")
      setNoticeContent("")
      toast.success(`已发送给 ${result.sent} 位用户`)
    },
    onError: (err) => toast.error(errorMessage(err, "发送失败")),
  })

  function saveShoppingCategory() {
    const itemName = shoppingItemName.trim()
    if (!itemName) {
      toast.error("请输入食材名称")
      return
    }
    saveCategoryMut.mutate({ item_name: itemName, category: shoppingCategory })
  }

  if (loadError) return <div><RequestState error={loadError} onRetry={() => { void retryLoad() }} /></div>

  if (isLoading) return <div className="p-8 text-center text-text2">加载中...</div>

  return (
    <div className="px-5 py-4 max-w-[640px] mx-auto pb-20 space-y-4">
      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">基本设置</div>
        <div className="text-[11px] text-text3 mb-2">应用名称与显示偏好</div>

        <SettingRow label="应用名称" stacked>
          <div className="flex w-full min-w-0 items-center gap-2">
            <input
              type="text"
              aria-label="应用名称"
              value={appName !== null ? appName : asString(settings?.app_name, storedAppName)}
              onChange={(e) => setAppName(e.target.value)}
              placeholder="arre食谱推荐小助手"
              className="min-w-0 flex-1 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
            />
            <button
              onClick={() => updateMut.mutate({ app_name: (appName !== null ? appName : asString(settings?.app_name, "arre食谱推荐小助手")) || "arre食谱推荐小助手" })}
              disabled={updateMut.isPending}
              className="min-h-11 shrink-0 px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>

        <SettingRow label="推荐去重天数">
          <div className="flex items-center gap-2">
            <input
              type="text"
              inputMode="numeric"
              value={repeatDays !== null ? repeatDays : asString(settings?.repeat_days, "")}
              onChange={(e) => setRepeatDays(e.target.value.replace(/[^0-9]/g, ""))}
              placeholder="3"
              className="w-16 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary text-right"
            />
            <span className="text-xs text-text3 whitespace-nowrap">天内不重复</span>
            <button
              onClick={() => {
                const v = repeatDays !== null ? repeatDays : asString(settings?.repeat_days, "")
                if (!v || Number(v) < 1) { toast.error("至少需要1天"); return }
                updateMut.mutate({ repeat_days: v })
              }}
              disabled={updateMut.isPending}
              className="min-h-11 px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>



      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">站内信通知</div>
        <div className="text-[11px] text-text3 mb-3">发布后会按用户分别创建消息和已读状态，适合系统更新、功能上线、维护提醒等。</div>
        <div className="space-y-2">
          <select value={noticeType} onChange={(e) => setNoticeType(e.target.value)} className="h-10 w-full rounded-[10px] border border-border bg-bg px-3 text-sm outline-none focus:border-primary">
            <option value="system_update">系统更新</option>
            <option value="feature">新功能上线</option>
            <option value="maintenance">维护提醒</option>
            <option value="health_tip">健康饮食提示</option>
            <option value="agent_security">Agent 安全提醒</option>
            <option value="family">家庭功能提醒</option>
          </select>
          <input value={noticeTitle} onChange={(e) => setNoticeTitle(e.target.value)} maxLength={128} placeholder="通知标题" className="h-10 w-full rounded-[10px] border border-border bg-bg px-3 text-sm outline-none focus:border-primary" />
          <textarea value={noticeContent} onChange={(e) => setNoticeContent(e.target.value)} maxLength={5000} rows={4} placeholder="通知内容" className="w-full resize-y rounded-[10px] border border-border bg-bg px-3 py-2 text-sm outline-none focus:border-primary" />
          <button onClick={() => noticeMut.mutate()} disabled={!noticeTitle.trim() || !noticeContent.trim() || noticeMut.isPending} className="h-10 rounded-full bg-primary px-4 text-xs font-semibold text-white disabled:opacity-50">{noticeMut.isPending ? "发送中…" : "广播给全部用户"}</button>
        </div>
      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <h2 className="text-sm font-semibold text-text">AI 助手请求上限</h2>
        <p className="mt-1 text-xs leading-relaxed text-text3">每个账号每天的提问次数，Web 与小程序共用。北京时间 00:00 重置，新建或删除对话不会清零。</p>
        <SettingRow label="每人每日最多" stacked>
          <div className="flex flex-wrap items-center gap-2">
            <input type="number" inputMode="numeric" min={0} max={20} step={1} aria-label="AI 助手每日请求上限"
              value={assistantLimit ?? asString(settings?.assistant_daily_limit, "20")}
              onChange={(event) => setAssistantLimit(event.target.value)}
              className="min-h-11 w-20 rounded-[10px] border border-border bg-bg px-3 text-sm outline-none focus:border-primary" />
            <span className="text-sm text-text2">次</span>
            <button disabled={updateMut.isPending} onClick={() => {
              const value = assistantLimit ?? asString(settings?.assistant_daily_limit, "20")
              if (!/^\d+$/.test(value) || Number(value) > 20) { toast.error("请输入 0–20 之间的整数"); return }
              updateMut.mutate({ assistant_daily_limit: String(Number(value)) })
            }} className="min-h-11 rounded-full bg-primary px-4 text-xs font-semibold text-white disabled:opacity-60">保存请求上限</button>
          </div>
        </SettingRow>
        <p className="text-xs leading-relaxed text-text3">默认 20 次，最高 20 次；设为 0 可暂停请求。每条被助手接收的消息计 1 次，停止生成也会计入。调整上限立即生效，已用次数保留。</p>
        <SettingRow label="全站每日总额度" stacked>
          <div className="flex flex-wrap items-center gap-2">
            <input type="number" inputMode="numeric" min={0} max={10000} step={1} aria-label="全站每日 AI 总额度"
              value={siteLimit ?? asString(settings?.assistant_site_daily_limit, "200")}
              onChange={(event) => setSiteLimit(event.target.value)}
              className="min-h-11 w-24 rounded-[10px] border border-border bg-bg px-3 text-sm outline-none focus:border-primary" />
            <span className="text-sm text-text2">次</span>
            <button disabled={updateMut.isPending} onClick={() => {
              const value = siteLimit ?? asString(settings?.assistant_site_daily_limit, "200")
              if (!/^\d+$/.test(value) || Number(value) > 10000) { toast.error("请输入 0–10000 之间的整数"); return }
              updateMut.mutate({ assistant_site_daily_limit: String(Number(value)) })
            }} className="min-h-11 rounded-full bg-primary px-4 text-xs font-semibold text-white disabled:opacity-60">保存全站额度</button>
          </div>
        </SettingRow>
        <p className="text-xs leading-relaxed text-text3">默认 200 次，用于控制全部账号的总消耗；设为 0 可暂停服务。单个账号仍最多 20 次。每人同时只处理 1 个问题，全站同时最多 4 个。</p>
        <p className="mt-3 border-t border-border pt-3 text-xs leading-relaxed text-text3">内容检查始终启用：仅处理食谱与日常饮食；提问和回复经过检查后才进入下一步。资料或菜谱中的指令不能授权后台操作，删除操作需在对应页面确认。</p>
      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">一周菜单</div>
        <div className="text-[11px] text-text3 mb-2">设定每日午/晚餐推荐菜品数量</div>

        <SettingRow label="每日午餐数量">
          <div className="flex items-center gap-2">
            <input
              type="text"
              inputMode="numeric"
              value={lunchPerDay !== null ? lunchPerDay : asString(settings?.lunch_dishes_per_day, "")}
              onChange={(e) => setLunchPerDay(e.target.value.replace(/[^0-9]/g, ""))}
              placeholder="1"
              className="w-16 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary text-right"
            />
            <span className="text-xs text-text3">道菜</span>
            <button
              onClick={() => {
                const v = lunchPerDay !== null ? lunchPerDay : asString(settings?.lunch_dishes_per_day, "")
                if (!v || Number(v) < 1) { toast.error("至少需要1道菜"); return }
                updateMut.mutate({ lunch_dishes_per_day: v })
              }}
              disabled={updateMut.isPending}
              className="min-h-11 px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>

        <SettingRow label="每日晚餐数量">
          <div className="flex items-center gap-2">
            <input
              type="text"
              inputMode="numeric"
              value={dinnerPerDay !== null ? dinnerPerDay : asString(settings?.dinner_dishes_per_day, "")}
              onChange={(e) => setDinnerPerDay(e.target.value.replace(/[^0-9]/g, ""))}
              placeholder="1"
              className="w-16 py-2 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary text-right"
            />
            <span className="text-xs text-text3">道菜</span>
            <button
              onClick={() => {
                const v = dinnerPerDay !== null ? dinnerPerDay : asString(settings?.dinner_dishes_per_day, "")
                if (!v || Number(v) < 1) { toast.error("至少需要1道菜"); return }
                updateMut.mutate({ dinner_dishes_per_day: v })
              }}
              disabled={updateMut.isPending}
              className="min-h-11 px-3 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
            >
              保存
            </button>
          </div>
        </SettingRow>
      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">买菜分类修正</div>
        <div className="text-[11px] text-text3 mb-3">识别不准时，在这里固定食材分类；删除后恢复智能识别。</div>
        <div className="flex gap-2 mb-3">
          <input
            value={shoppingItemName}
            onChange={(e) => setShoppingItemName(e.target.value)}
            placeholder="食材名称，如番茄"
            className="flex-1 min-w-0 py-2.5 px-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
          />
          <select
            value={shoppingCategory}
            onChange={(e) => setShoppingCategory(e.target.value)}
            className="w-20 py-2.5 px-2 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary"
          >
            {shoppingCategories.map((category) => (
              <option key={category} value={category}>{category}</option>
            ))}
          </select>
          <button
            onClick={saveShoppingCategory}
            disabled={saveCategoryMut.isPending}
            className="px-4 rounded-full text-xs font-semibold bg-primary text-white disabled:opacity-60"
          >
            保存
          </button>
        </div>
        {categoryOverrides.length === 0 ? (
          <div className="text-xs text-text3 py-2">暂无手动修正</div>
        ) : (
          <div className="flex flex-col gap-1.5">
            {categoryOverrides.map((item: ShoppingCategoryOverride) => (
              <div key={item.item_name} className="flex items-center gap-2 py-1.5 border-t border-border first:border-t-0">
                <span className="text-sm font-medium text-text">{item.item_name}</span>
                <span className="text-[11px] text-primary bg-primary-light px-2 py-0.5 rounded-full">{item.category}</span>
                <button
                  onClick={() => {
                    setShoppingItemName(item.item_name)
                    setShoppingCategory(item.category)
                  }}
                  className="ml-auto text-[11px] text-text2 px-2 py-1 rounded-full bg-bg active:scale-95"
                >
                  修改
                </button>
                <button
                  onClick={() => deleteCategoryMut.mutate(item.item_name)}
                  disabled={deleteCategoryMut.isPending}
                  className="text-[11px] text-red px-2 py-1 rounded-full bg-bg active:scale-95 disabled:opacity-60"
                >
                  删除
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="rounded-2xl border border-border bg-card p-4">
        <div className="text-[13px] font-semibold text-text2 mb-1">账号安全</div>
        <div className="text-xs text-text3 leading-relaxed">
          邮箱验证码登录由服务端邮件配置提供。登录密码可在「我的 → 账号与安全」中管理；修改部署环境变量不会重置已有账号密码。
        </div>
      </div>
    </div>
  )
}
