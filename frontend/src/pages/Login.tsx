import { useEffect, useMemo, useRef, useState } from "react"
import { Navigate, useNavigate, useSearchParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { authApi, errorMessage } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import toast from "react-hot-toast"
import { ArrowRight, KeyRound, Loader2, Mail, ShieldCheck } from "lucide-react"

type Mode = "code" | "password"

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const QUICK_DOMAINS = ["qq.com", "foxmail.com", "163.com"]

function safeRedirect(raw: string | null): string {
  if (!raw || !raw.startsWith("/") || raw.startsWith("//") || raw.startsWith("/login")) return "/"
  return raw
}

export default function Login() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const redirect = safeRedirect(params.get("redirect"))
  const status = useAuthStore((s) => s.status)
  const setSession = useAuthStore((s) => s.setSession)
  const appName = useAppInfoStore((s) => s.appName)

  const [mode, setMode] = useState<Mode>("code")
  const [email, setEmail] = useState(() => {
    try {
      return localStorage.getItem("ninimenu_last_email") || ""
    } catch {
      return ""
    }
  })
  const [code, setCode] = useState("")
  const [account, setAccount] = useState("")
  const [password, setPassword] = useState("")
  const [cooldown, setCooldown] = useState(0)
  const [sending, setSending] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [codeSent, setCodeSent] = useState(false)
  const codeRef = useRef<HTMLInputElement>(null)

  const { data: options } = useQuery({ queryKey: ["auth-options"], queryFn: () => authApi.options(), staleTime: 60000 })

  useEffect(() => {
    if (cooldown <= 0) return
    const t = window.setTimeout(() => setCooldown((c) => c - 1), 1000)
    return () => window.clearTimeout(t)
  }, [cooldown])

  const emailValid = EMAIL_RE.test(email.trim())
  const domainHint = useMemo(() => {
    const v = email.trim()
    if (!v || v.includes("@") === false) return QUICK_DOMAINS
    const [, domain = ""] = v.split("@")
    if (domain.includes(".")) return []
    return QUICK_DOMAINS.filter((d) => d.startsWith(domain))
  }, [email])

  if (status === "authenticated") return <Navigate to={redirect} replace />

  async function sendCode() {
    if (!emailValid || cooldown > 0 || sending) return
    setSending(true)
    try {
      const res = await authApi.sendEmailCode(email.trim())
      setCooldown(res.cooldown || 60)
      setCodeSent(true)
      try {
        localStorage.setItem("ninimenu_last_email", email.trim())
      } catch {
        /* ignore */
      }
      toast.success(options?.email_dev ? "开发模式：验证码已打印在服务端日志" : "验证码已发送，请查收邮件")
      window.setTimeout(() => codeRef.current?.focus(), 50)
    } catch (err) {
      const data = (err as { data?: { cooldown?: number } }).data
      if (data?.cooldown) setCooldown(data.cooldown)
      toast.error(errorMessage(err, "发送失败"))
    } finally {
      setSending(false)
    }
  }

  async function submit() {
    if (submitting) return
    setSubmitting(true)
    try {
      const res =
        mode === "code"
          ? await authApi.emailLogin(email.trim(), code.trim())
          : await authApi.passwordLogin(account.trim(), password)
      setSession(res.token, res.user)
      toast.success(res.created ? `欢迎加入 ${appName} 🎉` : `欢迎回来，${res.user.nickname}`)
      navigate(redirect, { replace: true })
    } catch (err) {
      toast.error(errorMessage(err, "登录失败"))
    } finally {
      setSubmitting(false)
    }
  }

  const canSubmit = mode === "code" ? emailValid && /^\d{6}$/.test(code.trim()) : account.trim() !== "" && password !== ""

  return (
    <div className="relative min-h-dvh overflow-hidden bg-bg">
      {/* 背景：暖色光晕 */}
      <div className="pointer-events-none absolute -left-24 -top-32 h-80 w-80 rounded-full bg-primary/25 blur-3xl" />
      <div className="pointer-events-none absolute -right-20 top-40 h-72 w-72 rounded-full bg-yellow/30 blur-3xl" />
      <div className="pointer-events-none absolute bottom-0 left-1/3 h-64 w-64 rounded-full bg-mint/20 blur-3xl" />

      <div className="relative mx-auto flex min-h-dvh max-w-[420px] flex-col px-6" style={{ paddingTop: "calc(56px + env(safe-area-inset-top))", paddingBottom: "calc(24px + env(safe-area-inset-bottom))" }}>
        <div className="mb-8">
          <img src="/chef-mark.svg" alt={appName} className="mb-5 h-[68px] w-[68px] rounded-[20px] shadow-[0_14px_36px_rgba(232,115,74,.28)]" />
          <h1 className="text-[30px] font-black leading-tight tracking-tight text-text">
            今天吃什么，
            <br />
            <span className="text-primary">交给{appName}</span>
          </h1>
          <p className="mt-2.5 text-[14px] leading-relaxed text-text2">按你的口味推荐每一餐，AI 助手帮你排菜单、记饮食、列清单。</p>
        </div>

        <div className="rounded-[28px] border border-white/60 bg-card/90 p-5 shadow-[0_24px_60px_rgba(26,26,46,.10)] backdrop-blur-xl">
          <div className="mb-5 grid grid-cols-2 gap-1 rounded-full bg-bg p-1">
            {([
              { key: "code", label: "邮箱验证码", Icon: Mail },
              { key: "password", label: "密码登录", Icon: KeyRound },
            ] as const).map(({ key, label, Icon }) => (
              <button
                key={key}
                onClick={() => setMode(key)}
                className={`flex h-9 items-center justify-center gap-1.5 rounded-full text-[13px] font-bold transition-all ${mode === key ? "bg-card text-text shadow-sm" : "text-text3"}`}
              >
                <Icon size={15} strokeWidth={2.4} />
                {label}
              </button>
            ))}
          </div>

          {mode === "code" ? (
            <div className="space-y-3">
              <label className="block">
                <span className="mb-1.5 block text-[12px] font-bold text-text2">邮箱</span>
                <input
                  type="email"
                  inputMode="email"
                  autoComplete="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="你的 QQ 邮箱，如 123456@qq.com"
                  className="h-12 w-full rounded-2xl border-[1.5px] border-border bg-bg px-4 text-[15px] outline-none transition-all focus:border-primary focus:bg-card focus:shadow-[0_0_0_4px_rgba(232,115,74,.10)]"
                />
              </label>
              {domainHint.length > 0 && email.trim() !== "" && !emailValid && (
                <div className="-mt-1 flex flex-wrap gap-1.5">
                  {domainHint.map((d) => (
                    <button
                      key={d}
                      onClick={() => setEmail(`${email.split("@")[0]}@${d}`)}
                      className="rounded-full bg-primary-light px-2.5 py-1 text-[11px] font-semibold text-primary active:scale-95"
                    >
                      @{d}
                    </button>
                  ))}
                </div>
              )}
              <label className="block">
                <span className="mb-1.5 block text-[12px] font-bold text-text2">验证码</span>
                <div className="flex gap-2">
                  <input
                    ref={codeRef}
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                    onKeyDown={(e) => e.key === "Enter" && canSubmit && submit()}
                    placeholder="6 位数字"
                    className="h-12 min-w-0 flex-1 rounded-2xl border-[1.5px] border-border bg-bg px-4 text-[18px] font-bold tracking-[0.3em] outline-none transition-all placeholder:text-[14px] placeholder:font-normal placeholder:tracking-normal focus:border-primary focus:bg-card focus:shadow-[0_0_0_4px_rgba(232,115,74,.10)]"
                  />
                  <button
                    onClick={sendCode}
                    disabled={!emailValid || cooldown > 0 || sending}
                    className="h-12 w-[112px] shrink-0 rounded-2xl bg-primary-light text-[13px] font-bold text-primary transition-all active:scale-95 disabled:opacity-50"
                  >
                    {sending ? <Loader2 size={16} className="mx-auto animate-spin" /> : cooldown > 0 ? `${cooldown}s 后重发` : codeSent ? "重新发送" : "获取验证码"}
                  </button>
                </div>
              </label>
              <p className="text-[11px] leading-relaxed text-text3">
                未注册的邮箱验证后自动创建账号
                {options?.email_domains && options.email_domains.length > 0 ? `（支持 ${options.email_domains.map((d) => "@" + d).join("、")}）` : ""}
                {options && !options.register_open ? "；当前暂停新用户注册" : ""}
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              <label className="block">
                <span className="mb-1.5 block text-[12px] font-bold text-text2">账号</span>
                <input
                  autoComplete="username"
                  value={account}
                  onChange={(e) => setAccount(e.target.value)}
                  placeholder="邮箱或用户名"
                  className="h-12 w-full rounded-2xl border-[1.5px] border-border bg-bg px-4 text-[15px] outline-none transition-all focus:border-primary focus:bg-card focus:shadow-[0_0_0_4px_rgba(232,115,74,.10)]"
                />
              </label>
              <label className="block">
                <span className="mb-1.5 block text-[12px] font-bold text-text2">密码</span>
                <input
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && canSubmit && submit()}
                  placeholder="在「我的 → 账号与安全」设置过密码才可用"
                  className="h-12 w-full rounded-2xl border-[1.5px] border-border bg-bg px-4 text-[15px] outline-none transition-all focus:border-primary focus:bg-card focus:shadow-[0_0_0_4px_rgba(232,115,74,.10)]"
                />
              </label>
            </div>
          )}

          <button
            onClick={submit}
            disabled={!canSubmit || submitting}
            className="mt-5 flex h-12 w-full items-center justify-center gap-2 rounded-2xl bg-primary text-[15px] font-extrabold text-white shadow-[0_12px_28px_rgba(232,115,74,.32)] transition-all hover:bg-primary-dark active:scale-[.98] disabled:opacity-50 disabled:shadow-none"
          >
            {submitting ? <Loader2 size={18} className="animate-spin" /> : <>登录 <ArrowRight size={18} strokeWidth={2.6} /></>}
          </button>
        </div>

        <div className="mt-auto flex items-center justify-center gap-1.5 pt-8 text-[11px] text-text3">
          <ShieldCheck size={13} strokeWidth={2.4} />
          你的饮食数据只属于你，AI 也只能读取你授权的部分
        </div>
      </div>
    </div>
  )
}
