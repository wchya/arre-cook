import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Search, Shield, UserRound, Users, UserX } from "lucide-react"
import toast from "react-hot-toast"
import { adminApi, errorMessage } from "@/api"
import PageHeader from "@/components/PageHeader"
import { useAuthStore } from "@/store/useAuthStore"

export default function AdminUsers() {
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.user)
  const [input, setInput] = useState("")
  const [search, setSearch] = useState("")
  const [page, setPage] = useState(1)
  const { data, isLoading, isError } = useQuery({ queryKey: ["admin-users", search, page], queryFn: () => adminApi.users({ search, page: String(page), page_size: "20" }) })
  const updateMutation = useMutation({
    mutationFn: ({ id, disabled }: { id: number; disabled: boolean }) => adminApi.updateUser(id, { disabled }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin-users"] })
      toast.success("账号状态已更新")
    },
    onError: (error) => toast.error(errorMessage(error, "更新失败")),
  })

  return (
    <div>
      <PageHeader title="用户" subtitle="管理账号状态与使用情况" icon={Users} />
      <main className="mx-auto max-w-[960px] px-4 py-5">
        <form onSubmit={(event) => { event.preventDefault(); setPage(1); setSearch(input.trim()) }} className="mb-4 flex gap-2">
          <label className="flex h-10 min-w-0 flex-1 items-center gap-2 rounded-md border border-border bg-card px-3"><Search size={15} className="shrink-0 text-text3" /><input value={input} onChange={(event) => setInput(event.target.value)} placeholder="按邮箱、昵称或用户名搜索" className="min-w-0 flex-1 bg-transparent text-sm outline-none" /></label>
          <button className="h-10 rounded-md bg-text px-4 text-xs font-semibold text-white">搜索</button>
        </form>
        <div className="mb-2 flex items-center justify-between text-xs text-text3"><span>共 {data?.total ?? 0} 个账号</span><span>每页 20 个</span></div>
        {isLoading ? <div className="h-32 animate-pulse rounded-md bg-card" /> : isError ? <div role="alert" className="rounded-md border border-border bg-card p-5 text-sm text-text2">用户列表加载失败，请稍后重试。</div> : (
          <div className="overflow-hidden rounded-md border border-border bg-card">
            <div className="hidden grid-cols-[minmax(0,1fr)_110px_100px_130px] gap-4 border-b border-border bg-bg px-4 py-2.5 text-[11px] font-semibold text-text3 md:grid"><span>账号</span><span>角色</span><span>用餐记录</span><span>状态</span></div>
            {data?.items.map((user) => <article key={user.id} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 border-b border-border px-4 py-3 last:border-0 md:grid-cols-[minmax(0,1fr)_110px_100px_130px] md:gap-4">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2"><span className="truncate text-sm font-semibold">{user.nickname || user.username || "未命名用户"}</span>{user.id === currentUser?.id && <span className="shrink-0 rounded bg-bg px-1.5 py-0.5 text-[10px] text-text3">当前账号</span>}</div>
                <div className="truncate text-[11px] text-text3">{user.email || user.username}</div>
                <div className="mt-1 text-[11px] text-text3 md:hidden">{user.role === "admin" ? "管理员" : "用户"} · {user.record_count} 条记录</div>
              </div>
              <span className="hidden items-center gap-1 text-xs text-text2 md:flex">{user.role === "admin" ? <><Shield size={13} />管理员</> : <><UserRound size={13} />用户</>}</span>
              <span className="hidden text-xs tabular-nums text-text2 md:inline">{user.record_count}</span>
              <button onClick={() => { const disabled = !user.disabled; if (confirm(`${disabled ? "停用" : "启用"} ${user.nickname || user.email || "此账号"}？`)) updateMutation.mutate({ id: user.id, disabled }) }} disabled={user.id === currentUser?.id || updateMutation.isPending} className={`flex h-8 items-center justify-center gap-1.5 rounded-md border px-2 text-[11px] font-semibold disabled:opacity-40 ${user.disabled ? "border-mint/40 text-mint" : "border-border text-text2 hover:bg-bg"}`}>
                <UserX size={13} />{user.disabled ? "已停用 · 启用" : "正常 · 停用"}
              </button>
            </article>)}
            {data?.items.length === 0 && <div className="px-4 py-10 text-center text-sm text-text3">没有匹配的账号</div>}
          </div>
        )}
        <div className="mt-4 flex items-center justify-between">
          <button onClick={() => setPage((value) => Math.max(1, value - 1))} disabled={page <= 1} className="h-9 rounded-md border border-border px-3 text-xs font-semibold disabled:opacity-40">上一页</button>
          <span className="text-xs tabular-nums text-text3">第 {page} / {Math.max(1, Math.ceil((data?.total ?? 0) / 20))} 页</span>
          <button onClick={() => setPage((value) => value + 1)} disabled={page * 20 >= (data?.total ?? 0)} className="h-9 rounded-md border border-border px-3 text-xs font-semibold disabled:opacity-40">下一页</button>
        </div>
      </main>
    </div>
  )
}
