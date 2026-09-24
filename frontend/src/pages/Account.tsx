import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation } from "@tanstack/react-query"
import { Download, KeyRound, LogOut, ShieldCheck, Trash2 } from "lucide-react"
import toast from "react-hot-toast"
import { errorMessage, meApi } from "@/api"
import PageHeader from "@/components/PageHeader"
import { useAuthStore } from "@/store/useAuthStore"

export default function Account() {
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.user)
  const setSession = useAuthStore((state) => state.setSession)
  const logout = useAuthStore((state) => state.logout)
  const [oldPassword, setOldPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [deleteConfirm, setDeleteConfirm] = useState("")
  const passwordMutation = useMutation({
    mutationFn: () => meApi.changePassword({ old_password: oldPassword, new_password: newPassword }),
    onSuccess: (result) => {
      setSession(result.token, result.user)
      setOldPassword("")
      setNewPassword("")
      setConfirmPassword("")
      toast.success("密码已更新")
    },
    onError: (error) => toast.error(errorMessage(error, "密码更新失败")),
  })
  const logoutAllMutation = useMutation({
    mutationFn: () => meApi.logoutAll(),
    onSuccess: () => {
      void logout()
      navigate("/login", { replace: true })
    },
    onError: (error) => toast.error(errorMessage(error, "退出其他设备失败")),
  })
  const deleteMutation = useMutation({
    mutationFn: () => meApi.remove(deleteConfirm),
    onSuccess: async () => {
      await logout()
      navigate("/login", { replace: true })
    },
    onError: (error) => toast.error(errorMessage(error, "注销失败")),
  })

  async function exportData() {
    try {
      const data = await meApi.export()
      const file = new Blob([JSON.stringify(data, null, 2)], { type: "application/json;charset=utf-8" })
      const url = URL.createObjectURL(file)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = `nini-menu-data-${new Date().toISOString().slice(0, 10)}.json`
      anchor.click()
      URL.revokeObjectURL(url)
    } catch (error) {
      toast.error(errorMessage(error, "导出失败"))
    }
  }

  const passwordValid = newPassword.length >= 8 && newPassword === confirmPassword
  return (
    <div className="min-h-dvh bg-bg pb-8">
      <PageHeader title="账号与安全" subtitle="登录方式、数据导出与账号管理" icon={ShieldCheck} onBack={() => navigate(-1)} />
      <main className="mx-auto max-w-[640px] space-y-4 px-5 py-5">
        <section className="rounded-lg border border-border bg-card p-4">
          <h2 className="text-sm font-bold">登录邮箱</h2>
          <p className="mt-1 break-all text-sm text-text2">{user?.email || "未绑定邮箱"}</p>
          <p className="mt-2 text-xs leading-5 text-text3">验证码登录始终可用。设置密码后，也可以用邮箱和密码登录。</p>
        </section>

        <section className="rounded-lg border border-border bg-card p-4">
          <h2 className="flex items-center gap-2 text-sm font-bold"><KeyRound size={16} />{user?.has_password ? "修改密码" : "设置密码"}</h2>
          {user?.has_password && <label className="mt-4 block text-xs font-semibold text-text2">当前密码<input type="password" autoComplete="current-password" value={oldPassword} onChange={(event) => setOldPassword(event.target.value)} className="mt-1.5 h-11 w-full rounded-md border border-border bg-bg px-3 text-sm outline-none focus:border-primary" /></label>}
          <label className="mt-4 block text-xs font-semibold text-text2">新密码<input type="password" autoComplete="new-password" minLength={8} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} className="mt-1.5 h-11 w-full rounded-md border border-border bg-bg px-3 text-sm outline-none focus:border-primary" /></label>
          <label className="mt-3 block text-xs font-semibold text-text2">确认新密码<input type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} className="mt-1.5 h-11 w-full rounded-md border border-border bg-bg px-3 text-sm outline-none focus:border-primary" /></label>
          {confirmPassword && newPassword !== confirmPassword && <p role="alert" className="mt-2 text-xs text-red">两次输入的密码不一致</p>}
          <button onClick={() => passwordMutation.mutate()} disabled={!passwordValid || passwordMutation.isPending || (Boolean(user?.has_password) && !oldPassword)} className="mt-4 h-10 rounded-md bg-text px-4 text-xs font-semibold text-white disabled:opacity-40">{passwordMutation.isPending ? "保存中…" : "保存密码"}</button>
        </section>

        <section className="rounded-lg border border-border bg-card p-4">
          <h2 className="text-sm font-bold">个人数据</h2>
          <p className="mt-1 text-xs leading-5 text-text3">导出包含用餐记录、私房菜、偏好、AI 对话和建议。</p>
          <button onClick={() => void exportData()} className="mt-3 flex h-10 items-center gap-2 rounded-md border border-border px-3 text-xs font-semibold text-text2 hover:bg-bg"><Download size={15} />导出我的数据</button>
        </section>

        <section className="rounded-lg border border-border bg-card p-4">
          <h2 className="text-sm font-bold">登录设备</h2>
          <p className="mt-1 text-xs leading-5 text-text3">让其他设备上的网页登录失效；当前设备也会退出。</p>
          <button onClick={() => { if (confirm("退出所有设备？你需要重新登录。")) logoutAllMutation.mutate() }} disabled={logoutAllMutation.isPending} className="mt-3 flex h-10 items-center gap-2 rounded-md border border-border px-3 text-xs font-semibold text-text2 hover:bg-bg disabled:opacity-50"><LogOut size={15} />退出所有设备</button>
        </section>

        <section className="rounded-lg border border-red/25 bg-card p-4">
          <h2 className="flex items-center gap-2 text-sm font-bold text-red"><Trash2 size={15} />注销账号</h2>
          <p className="mt-1 text-xs leading-5 text-text3">账号和个人数据会被永久删除，公共菜谱不会删除。此操作无法撤销。</p>
          <label className="mt-3 block text-xs font-semibold text-text2">输入“注销”确认<input value={deleteConfirm} onChange={(event) => setDeleteConfirm(event.target.value)} className="mt-1.5 h-11 w-full rounded-md border border-border bg-bg px-3 text-sm outline-none focus:border-red" /></label>
          <button onClick={() => deleteMutation.mutate()} disabled={deleteConfirm.trim() !== "注销" || deleteMutation.isPending} className="mt-3 h-10 rounded-md bg-red px-4 text-xs font-semibold text-white disabled:opacity-40">{deleteMutation.isPending ? "正在注销…" : "永久注销账号"}</button>
        </section>
      </main>
    </div>
  )
}
