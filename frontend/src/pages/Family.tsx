import { useMemo, useState } from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowRight, Check, Copy, Plus, Trash2, UsersRound } from "lucide-react"
import toast from "react-hot-toast"
import { dishesApi, errorMessage, familyApi } from "@/api"
import PageHeader from "@/components/PageHeader"
import { copyText } from "@/lib/clipboard"
import type { Dish } from "@/types"

type Tab = "members" | "plan" | "shopping" | "dishes"
const tabs: { id: Tab; label: string }[] = [
  { id: "members", label: "成员" }, { id: "plan", label: "菜单" },
  { id: "shopping", label: "买菜" }, { id: "dishes", label: "菜谱" },
]
const mealNames: Record<string, string> = { lunch: "午餐", dinner: "晚餐" }

function localDate(offset: number) {
  const date = new Date()
  date.setDate(date.getDate() + offset)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
}

export default function Family() {
  const navigate = useNavigate()
  const location = useLocation()
  const qc = useQueryClient()
  const token = new URLSearchParams(location.hash.slice(1)).get("invite") || ""
  const [tab, setTab] = useState<Tab>("members")
  const [busy, setBusy] = useState(false)
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [inviteLink, setInviteLink] = useState("")
  const [itemName, setItemName] = useState("")
  const [amount, setAmount] = useState("")
  const [search, setSearch] = useState("")
  const { data, isLoading } = useQuery({ queryKey: ["family"], queryFn: familyApi.get })
  const hasFamily = !!data?.family
  const isOwner = data?.role === "owner"
  const { data: plan = [] } = useQuery({ queryKey: ["family-plan"], queryFn: () => familyApi.plan(), enabled: hasFamily })
  const { data: shopping = [] } = useQuery({ queryKey: ["family-shopping"], queryFn: familyApi.shopping, enabled: hasFamily })
  const { data: shared } = useQuery({ queryKey: ["family-dishes"], queryFn: () => dishesApi.list({ scope: "family", pageSize: "100" }), enabled: hasFamily })
  const { data: privateDishes } = useQuery({ queryKey: ["my-dishes-family"], queryFn: () => dishesApi.list({ scope: "mine", pageSize: "100" }), enabled: hasFamily && tab === "dishes" })
  const { data: publicDishes } = useQuery({ queryKey: ["public-dishes-family", search], queryFn: () => dishesApi.list({ scope: "public", search, pageSize: "100" }), enabled: hasFamily && tab === "plan" })
  const choices = useMemo(() => [...(shared?.items || []), ...(publicDishes?.items || [])], [shared, publicDishes])
  const dates = useMemo(() => Array.from({ length: 7 }, (_, i) => localDate(i)), [])

  async function run(action: () => Promise<unknown>, message: string): Promise<boolean> {
    if (busy) return false
    setBusy(true)
    try {
      await action()
      await Promise.all(["family", "family-plan", "family-shopping", "family-dishes", "my-dishes-family", "public-dishes-family", "dishes"].map((key) => qc.invalidateQueries({ queryKey: [key] })))
      toast.success(message)
      return true
    } catch (err) {
      toast.error(errorMessage(err))
      return false
    } finally {
      setBusy(false)
    }
  }

  async function join() {
    if (await run(() => familyApi.join(token), "已加入家庭")) navigate("/family", { replace: true })
  }

  async function invite() {
    if (!email.trim() || busy) return
    setBusy(true)
    try {
      const res = await familyApi.invite(email.trim())
      setInviteLink(res.link)
      setEmail("")
      await qc.invalidateQueries({ queryKey: ["family"] })
      toast.success(res.sent ? "邀请邮件已发送" : "邀请已创建，请复制链接发给对方")
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="min-h-dvh bg-bg pb-12 text-text">
      <PageHeader title="我的家庭" subtitle={data?.family?.name || "一起安排每一餐"} icon={UsersRound} onBack={() => navigate(-1)} />
      <main className="mx-auto max-w-[640px] px-5 py-5">
        {isLoading ? <div className="skeleton h-40 rounded-lg" /> : !hasFamily ? (
          <div className="space-y-7">
            {token && <section className="border-b border-border pb-6">
              <h2 className="mb-3 text-[17px] font-bold">收到家庭邀请</h2>
              <p className="mb-4 text-sm text-text2">使用受邀邮箱登录后，即可加入并共享菜单与买菜清单。</p>
              <button disabled={busy} onClick={() => void join()} className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-primary font-semibold text-white disabled:opacity-50">接受邀请 <ArrowRight size={16} /></button>
            </section>}
            <section className="border-b border-border pb-6">
              <h2 className="mb-3 text-[17px] font-bold">创建家庭</h2>
              <div className="flex gap-2">
                <input value={name} onChange={(e) => setName(e.target.value)} maxLength={30} placeholder="家庭名称" className="h-11 min-w-0 flex-1 rounded-lg border border-border bg-card px-3 outline-none focus:border-primary" />
                <button disabled={!name.trim() || busy} onClick={() => void run(() => familyApi.create(name.trim()), "家庭已创建")} className="h-11 rounded-lg bg-primary px-4 font-semibold text-white disabled:opacity-50">创建</button>
              </div>
            </section>
            {!token && <p className="text-sm leading-relaxed text-text3">已有家庭？请通过家庭创建者发送的邀请链接加入。</p>}
          </div>
        ) : (
          <>
            {token && <p className="mb-4 rounded-lg bg-yellow-light p-3 text-sm text-text2">你已加入家庭。需要加入其他家庭时，请先退出当前家庭。</p>}
            <div className="mb-5 grid grid-cols-4 border-b border-border" role="tablist" aria-label="家庭功能">
              {tabs.map((item) => <button key={item.id} role="tab" aria-selected={tab === item.id} onClick={() => setTab(item.id)} className={`h-11 border-b-2 text-sm font-semibold ${tab === item.id ? "border-primary text-primary" : "border-transparent text-text3"}`}>{item.label}</button>)}
            </div>

            {tab === "members" && <div className="space-y-6">
              {isOwner && <form onSubmit={(e) => { e.preventDefault(); const nextName = String(new FormData(e.currentTarget).get("name") || "").trim(); if (nextName && nextName !== data?.family?.name) void run(() => familyApi.rename(nextName), "家庭名称已更新") }} className="border-b border-border pb-5"><label className="text-xs font-semibold text-text3">家庭名称<div className="mt-2 flex gap-2"><input key={data?.family?.id} name="name" defaultValue={data?.family?.name} maxLength={30} className="h-11 min-w-0 flex-1 rounded-lg border border-border bg-card px-3 text-sm text-text" /><button disabled={busy} className="h-11 rounded-lg border border-border px-4 text-sm font-semibold disabled:opacity-50">保存</button></div></label></form>}
              <section>
                <div className="mb-3 flex items-center justify-between"><h2 className="text-base font-bold">家庭成员</h2><span className="text-xs text-text3">{data?.members.length || 0}/12</span></div>
                <div className="divide-y divide-border border-y border-border">
                  {data?.members.map((member) => <div key={member.user_id} className="flex min-h-14 items-center gap-3 py-2">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-mint-light font-bold text-mint">{member.nickname.slice(0, 1)}</div>
                    <div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{member.nickname} {member.role === "owner" && <span className="text-xs text-primary">创建者</span>}</div><div className="truncate text-xs text-text3">{member.email}</div></div>
                    {isOwner && member.role !== "owner" && <button title="移除成员" aria-label={`移除${member.nickname}`} onClick={() => confirm(`移除 ${member.nickname}？`) && void run(() => familyApi.kick(member.user_id), "成员已移除")} className="p-2 text-text3"><Trash2 size={16} /></button>}
                  </div>)}
                </div>
              </section>
              {isOwner && <section>
                <h2 className="mb-3 text-base font-bold">邀请成员</h2>
                <div className="flex gap-2"><input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="对方的邮箱" className="h-11 min-w-0 flex-1 rounded-lg border border-border bg-card px-3 outline-none focus:border-primary" /><button disabled={!email.trim() || busy} onClick={() => void invite()} className="h-11 rounded-lg bg-primary px-4 font-semibold text-white disabled:opacity-50">邀请</button></div>
                {inviteLink && <button onClick={() => void copyText(inviteLink, "邀请链接已复制")} className="mt-2 flex items-center gap-2 text-sm text-primary"><Copy size={15} />复制邀请链接</button>}
                {(data?.invitations.length || 0) > 0 && <div className="mt-4 border-t border-border pt-3"><div className="mb-1 text-xs font-semibold text-text3">待接受邀请</div>{data?.invitations.map((invite) => <div key={invite.id} className="flex items-center justify-between py-2 text-sm"><span className="truncate">{invite.email}</span><button onClick={() => void run(() => familyApi.revoke(invite.id), "邀请已撤销")} className="shrink-0 px-2 text-xs text-red">撤销</button></div>)}</div>}
              </section>}
              <section className="border-t border-border pt-5">
                {isOwner ? <div className="space-y-3">
                  {data?.members.filter((m) => m.role !== "owner").length ? <label className="block text-xs text-text3">转让创建者身份<select defaultValue="" onChange={(e) => { const id = Number(e.target.value); if (id && confirm("确定转让家庭创建者身份？")) void run(() => familyApi.transfer(id), "已转让") }} className="mt-2 h-11 w-full rounded-lg border border-border bg-card px-3 text-sm text-text"><option value="">选择成员</option>{data?.members.filter((m) => m.role !== "owner").map((m) => <option key={m.user_id} value={m.user_id}>{m.nickname}</option>)}</select></label> : null}
                  <button onClick={() => confirm("解散后家庭菜单和买菜清单将删除，家庭菜谱也会移除。确定解散？") && void run(familyApi.remove, "家庭已解散")} className="text-sm text-red">解散家庭</button>
                </div> : <button onClick={() => confirm("确定退出家庭？") && void run(familyApi.leave, "已退出家庭")} className="text-sm text-red">退出家庭</button>}
              </section>
            </div>}

            {tab === "plan" && <div className="space-y-5">
              <p className="text-sm text-text3">每个餐次由家庭成员共同安排。菜谱选择仅包含公共菜谱和家庭菜谱。</p>
              <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索公共菜谱" className="h-11 w-full rounded-lg border border-border bg-card px-3 outline-none focus:border-primary" />
              <div className="divide-y divide-border border-y border-border">{dates.map((date) => <div key={date} className="py-3"><div className="mb-2 text-sm font-bold">{date.slice(5)} {date === localDate(0) ? "今天" : ""}</div><div className="grid grid-cols-1 gap-2 sm:grid-cols-2">{["lunch", "dinner"].map((meal) => {
                const selected = plan.find((p) => p.meal_date === date && p.meal_type === meal)?.dish.id || 0
                return <label key={meal} className="flex items-center gap-2 text-sm"><span className="w-9 shrink-0 text-text3">{mealNames[meal]}</span><select value={selected} disabled={busy} onChange={(e) => void run(() => familyApi.setPlan(date, meal, Number(e.target.value)), "家庭菜单已更新")} className="h-10 min-w-0 flex-1 rounded-lg border border-border bg-card px-2"><option value={0}>暂未安排</option>{selected && !choices.some((d) => d.id === selected) && <option value={selected}>{plan.find((p) => p.dish.id === selected)?.dish.name}</option>}{choices.map((dish) => <option key={dish.id} value={dish.id}>{dish.name}</option>)}</select></label>
              })}</div></div>)}</div>
            </div>}

            {tab === "shopping" && <div>
              <form onSubmit={(e) => { e.preventDefault(); if (itemName.trim()) void run(async () => { await familyApi.addShopping(itemName.trim(), amount.trim()); setItemName(""); setAmount("") }, "已加入买菜清单") }} className="mb-5 flex gap-2"><input value={itemName} onChange={(e) => setItemName(e.target.value)} placeholder="食材" className="h-11 min-w-0 flex-1 rounded-lg border border-border bg-card px-3" /><input value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="数量" className="h-11 w-20 rounded-lg border border-border bg-card px-2" /><button disabled={!itemName.trim() || busy} aria-label="添加食材" title="添加食材" className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-primary text-white disabled:opacity-50"><Plus size={20} /></button></form>
              {shopping.length === 0 ? <p className="py-8 text-center text-sm text-text3">买菜清单还没有食材</p> : <div className="divide-y divide-border border-y border-border">{shopping.map((item) => <div key={item.id} className="flex min-h-12 items-center gap-3"><button onClick={() => void run(() => familyApi.checkShopping(item.id, !item.checked), "已更新")} aria-label={item.checked ? "标记未购买" : "标记已购买"} className={`flex h-6 w-6 shrink-0 items-center justify-center rounded border ${item.checked ? "border-mint bg-mint text-white" : "border-border2"}`}>{item.checked && <Check size={15} />}</button><div className={`min-w-0 flex-1 truncate text-sm ${item.checked ? "text-text3 line-through" : ""}`}>{item.name} {item.amount}</div><button onClick={() => void run(() => familyApi.deleteShopping(item.id), "已删除")} title="删除" aria-label={`删除${item.name}`} className="p-2 text-text3"><Trash2 size={16} /></button></div>)}</div>}
            </div>}

            {tab === "dishes" && <div className="space-y-6">
              <section><h2 className="mb-2 text-base font-bold">家庭菜谱</h2>{!shared?.items.length ? <p className="text-sm text-text3">还没有共享的菜谱</p> : <div className="divide-y divide-border border-y border-border">{shared.items.map((dish) => <DishRow key={dish.id} dish={dish} onOpen={() => navigate(`/dishes/${dish.id}`)} action={<button disabled={busy} onClick={() => void run(() => familyApi.importIngredients(dish.id), "食材已加入家庭买菜清单")} className="shrink-0 text-xs font-semibold text-mint disabled:opacity-50">加入买菜清单</button>} />)}</div>}</section>
              <section><h2 className="mb-2 text-base font-bold">分享我的私房菜</h2><div className="divide-y divide-border border-y border-border">{privateDishes?.items.map((dish) => <DishRow key={dish.id} dish={dish} onOpen={() => navigate(`/dishes/${dish.id}`)} action={<button disabled={busy} onClick={() => void run(() => familyApi.shareDish(dish.id), "已复制到家庭菜谱")} className="shrink-0 text-xs font-semibold text-primary disabled:opacity-50">分享到家庭</button>} />)}</div>{!privateDishes?.items.length && <p className="text-sm text-text3">先创建私房菜，再分享给家庭。</p>}<button onClick={() => navigate("/dishes/new")} className="mt-3 flex items-center gap-1 text-sm font-semibold text-primary"><Plus size={16} />新建私房菜</button></section>
            </div>}
          </>
        )}
      </main>
    </div>
  )
}

function DishRow({ dish, onOpen, action }: { dish: Dish; onOpen: () => void; action?: React.ReactNode }) {
  return <div className="flex min-h-12 items-center gap-2 py-2"><button onClick={onOpen} className="min-w-0 flex-1 truncate text-left text-sm font-medium">{dish.name}</button>{action}<button onClick={onOpen} aria-label={`查看${dish.name}`} className="text-text3"><ArrowRight size={15} /></button></div>
}
