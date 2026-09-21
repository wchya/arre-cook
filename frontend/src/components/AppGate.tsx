import { useEffect, useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { authApi } from "@/api"
import { APP_TOKEN_KEY } from "@/api/client"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import AchievementUnlockOverlay from "@/components/AchievementUnlockOverlay"
import AiAssistant from "@/components/AiAssistant"
import toast from "react-hot-toast"

export default function AppGate({ children }: { children: React.ReactNode }) {
  const [unlocked, setUnlocked] = useState(() => !!localStorage.getItem(APP_TOKEN_KEY))
  const [password, setPassword] = useState("")
  const appName = useAppInfoStore((s) => s.appName)

  useEffect(() => {
    const onExpired = () => setUnlocked(false)
    window.addEventListener("app-auth-expired", onExpired)
    return () => window.removeEventListener("app-auth-expired", onExpired)
  }, [])

  const loginMut = useMutation({
    mutationFn: () => authApi.appLogin(password),
    onSuccess: () => {
      localStorage.setItem(APP_TOKEN_KEY, password)
      setUnlocked(true)
      setPassword("")
      toast.success("已进入")
    },
    onError: () => toast.error("应用密码错误"),
  })

  if (unlocked) {
    return (
      <>
        {children}
        <AchievementUnlockOverlay />
        <AiAssistant />
      </>
    )
  }

  return (
    <div className="min-h-screen bg-bg flex items-center justify-center px-5">
      <div className="w-full max-w-[360px] bg-card rounded-2xl border border-border p-5 shadow-[0_1px_3px_rgba(0,0,0,.04),0_8px_24px_rgba(0,0,0,.06)]">
        <div className="text-center mb-5">
          <img src="/180.png" alt={appName} className="w-[68px] h-[68px] mx-auto mb-2 rounded-[16px]" />
          <h1 className="text-xl font-bold tracking-tight">{appName}</h1>
          <p className="text-[13px] text-text2 mt-1">输入应用密码后进入点菜页面</p>
        </div>
        <label className="block text-[13px] font-semibold mb-1.5 text-text2">应用密码</label>
        <input
          type="password"
          value={password}
          placeholder="请输入应用密码"
          onChange={(e) => setPassword(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && password && loginMut.mutate()}
          className="w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm transition-all focus:border-primary focus:shadow-[0_0_0_3px_rgba(232,115,74,.1)] outline-none"
        />
        <button
          onClick={() => loginMut.mutate()}
          disabled={!password || loginMut.isPending}
          className="w-full mt-4 py-2.5 px-5 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96 hover:bg-primary-dark disabled:opacity-50"
        >
          {loginMut.isPending ? "验证中..." : "进入应用"}
        </button>
      </div>
    </div>
  )
}
