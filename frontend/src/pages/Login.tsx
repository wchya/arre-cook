import { useEffect, useMemo, useRef, useState } from "react"
import { Navigate, useNavigate, useSearchParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { authApi, errorMessage } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import toast from "react-hot-toast"
import AnimatedBottomSheet from "@/components/AnimatedBottomSheet"
import LegalContent from "@/components/LegalContent"
import ThemePicker from "@/components/ThemePicker"
import { ArrowRight, Eye, EyeOff, KeyRound, Loader2, LockKeyhole, Mail, MessageCircle, ShieldCheck, UserRound, X } from "lucide-react"

type Mode = "wechat" | "code" | "password"

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const QUICK_DOMAINS = ["qq.com", "foxmail.com", "163.com"]
type MiniProgramWx = { login: (options: { success: (result: { code: string }) => void; fail: () => void }) => void }

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

  const [mode, setMode] = useState<Mode>("wechat")
  const [agreed, setAgreed] = useState(false)
  const [legalType, setLegalType] = useState<"terms" | "privacy" | null>(null)
  const [focused, setFocused] = useState<"email" | "code" | "account" | "password" | "">("")
  const [passwordVisible, setPasswordVisible] = useState(false)
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

  const modes = useMemo(() => {
    const items: { key: Mode; label: string; Icon: typeof Mail }[] = []
    if (options?.wechat !== false) items.push({ key: "wechat", label: "微信登录", Icon: MessageCircle })
    if (options?.email !== false) items.push({ key: "code", label: "邮箱验证码", Icon: Mail })
    items.push({ key: "password", label: "密码登录", Icon: KeyRound })
    return items
  }, [options])

  useEffect(() => {
    if (!modes.some((item) => item.key === mode)) {
      // options is loaded asynchronously; keep the mini-program order once available.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setMode(modes[0]?.key || "password")
    }
  }, [mode, modes])

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
    if (!agreed || !emailValid || cooldown > 0 || sending) return
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
    if (submitting || !agreed) return
    setSubmitting(true)
    try {
      if (mode === "wechat") {
        const result = await new Promise<{ code: string }>((resolve, reject) => {
          const wx = (window as Window & { wx?: MiniProgramWx }).wx
          if (!wx?.login) return reject(new Error("请在微信小程序中使用微信登录"))
          wx.login({ success: resolve, fail: () => reject(new Error("无法获取微信登录凭证")) })
        })
        const wechat = await authApi.wechatLogin(result.code)
        if (wechat.need_bind) {
          toast("这个微信还未绑定，请使用邮箱验证码登录")
          setMode("code")
          return
        }
        setSession(wechat.token, wechat.user)
        navigate(redirect, { replace: true })
        return
      }
      const res = mode === "code" ? await authApi.emailLogin(email.trim(), code.trim()) : await authApi.passwordLogin(account.trim(), password)
      setSession(res.token, res.user)
      toast.success(res.created ? `欢迎加入 ${appName} 🎉` : `欢迎回来，${res.user.nickname}`)
      navigate(redirect, { replace: true })
    } catch (err) {
      toast.error(errorMessage(err, "登录失败"))
    } finally {
      setSubmitting(false)
    }
  }

  const canSubmit = mode === "wechat" || (mode === "code" ? emailValid && /^\d{6}$/.test(code.trim()) : account.trim() !== "" && password !== "")

  return (
    <div className="login-page app-scroll h-dvh overflow-x-hidden overflow-y-auto overscroll-y-contain bg-bg" style={{ touchAction: "pan-y" }}>
      <div className="relative mx-auto flex min-h-full max-w-[420px] flex-col px-6" style={{ paddingTop: "calc(20px + env(safe-area-inset-top))", paddingBottom: "calc(24px + env(safe-area-inset-bottom))" }}>
        <div className="login-appearance"><ThemePicker /></div>
        <div className="mb-8">
          <img src="/chef-mark.svg" alt={appName} className="mb-5 h-[68px] w-[68px] rounded-[20px]" />
          <h1 className="text-[30px] font-black leading-tight tracking-tight text-text">
            今天吃什么？
            <br />
            <span className="mt-2 block text-[22px] leading-snug text-primary">{appName}</span>
          </h1>
          <p className="mt-2.5 text-[14px] leading-relaxed text-text2">按你的口味推荐每一餐，AI 助手帮你排菜单、记饮食、列清单。</p>
        </div>

        <div className="login-panel">
          <div className="mb-5 grid gap-1 rounded-full bg-bg p-1" style={{ gridTemplateColumns: `repeat(${modes.length}, minmax(0, 1fr))` }}>
            {modes.map(({ key, label, Icon }) => (
              <button
                key={key}
                onClick={() => setMode(key)}
                className={`flex h-11 items-center justify-center gap-1.5 rounded-full text-[13px] font-bold transition-all ${mode === key ? "bg-card text-text shadow-sm" : "text-text3"}`}
              >
                <Icon size={15} strokeWidth={2.4} />
                {label}
              </button>
            ))}
          </div>

          {mode === "wechat" ? (
            <div className="flex flex-col items-center py-2 text-center">
              <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-[18px] bg-[#E8F8F0] text-[#07C160]"><MessageCircle size={28} /></div>
              <div className="text-lg font-extrabold text-text">微信一键登录</div>
              <p className="mt-1 text-xs text-text3">已绑定微信的账号可直接进入，无需输入密码</p>
              <button onClick={submit} disabled={submitting} className="mt-7 flex h-12 w-full items-center justify-center gap-2 rounded-2xl bg-[#07C160] text-[15px] font-extrabold text-white shadow-[0_12px_28px_rgba(7,193,96,.25)] disabled:opacity-50">
                {submitting ? <Loader2 size={18} className="animate-spin" /> : <><MessageCircle size={18} /> 微信一键登录</>}
              </button>
              {options?.email !== false && <button onClick={() => setMode("code")} className="mt-4 text-xs font-semibold text-primary">首次使用？用邮箱验证码登录并绑定微信 →</button>}
            </div>
          ) : mode === "code" ? (
            <div className="space-y-3">
              <label className="block">
                <span className="mb-1.5 block text-[12px] font-bold text-text2">邮箱</span>
                <div className={`flex h-12 items-center rounded-2xl border-[1.5px] bg-bg px-4 transition-all ${focused === "email" ? "border-primary bg-card shadow-[0_0_0_4px_rgba(232,115,74,.10)]" : "border-border"}`}>
                  <Mail size={17} strokeWidth={2.2} className={focused === "email" ? "text-primary" : "text-text3"} />
                  <input type="email" inputMode="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} onFocus={() => setFocused("email")} onBlur={() => setFocused("")} placeholder="你的 QQ 邮箱，如 123456@qq.com" className="ml-2 min-w-0 flex-1 bg-transparent text-[15px] outline-none placeholder:text-text3" />
                  {email && focused === "email" && <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => setEmail("")} className="mr-2 flex h-5 w-5 items-center justify-center rounded-full bg-text4 text-white"><X size={12} strokeWidth={3} /></button>}
                  <button type="button" onClick={sendCode} disabled={!agreed || !emailValid || cooldown > 0 || sending} className="-mr-2 flex h-8 min-w-[78px] shrink-0 items-center justify-center rounded-[11px] bg-primary-light px-2 text-[12px] font-bold text-primary disabled:opacity-50">
                    {sending ? <Loader2 size={14} className="animate-spin" /> : cooldown > 0 ? `${cooldown}s` : codeSent ? "重新发送" : "获取验证码"}
                  </button>
                </div>
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
                <div className="relative grid grid-cols-6 gap-2" onClick={() => codeRef.current?.focus()}>
                  {Array.from({ length: 6 }, (_, index) => {
                    const value = code[index]
                    const active = focused === "code" && index === code.length
                    return <div key={index} className={`flex h-12 items-center justify-center rounded-[13px] border-[1.5px] bg-bg text-[21px] font-extrabold transition-all ${value ? "border-border2 bg-card" : active ? "border-primary bg-card shadow-[0_0_0_4px_rgba(232,115,74,.10)]" : "border-border"}`}>{value || (active ? <span className="h-5 w-0.5 animate-pulse bg-primary" /> : null)}</div>
                  })}
                  <input ref={codeRef} inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))} onFocus={() => setFocused("code")} onBlur={() => setFocused("")} onKeyDown={(e) => e.key === "Enter" && canSubmit && submit()} className="absolute left-[-9999px] h-1 w-1 opacity-0" aria-label="6 位验证码" />
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
                <div className={`flex h-12 items-center rounded-2xl border-[1.5px] bg-bg px-4 transition-all ${focused === "account" ? "border-primary bg-card shadow-[0_0_0_4px_rgba(232,115,74,.10)]" : "border-border"}`}>
                  <UserRound size={17} strokeWidth={2.2} className={focused === "account" ? "text-primary" : "text-text3"} />
                  <input autoComplete="username" value={account} onChange={(e) => setAccount(e.target.value)} onFocus={() => setFocused("account")} onBlur={() => setFocused("")} placeholder="邮箱或用户名" className="ml-2 min-w-0 flex-1 bg-transparent text-[15px] outline-none placeholder:text-text3" />
                </div>
              </label>
              <label className="block">
                <span className="mb-1.5 block text-[12px] font-bold text-text2">密码</span>
                <div className={`flex h-12 items-center rounded-2xl border-[1.5px] bg-bg px-4 transition-all ${focused === "password" ? "border-primary bg-card shadow-[0_0_0_4px_rgba(232,115,74,.10)]" : "border-border"}`}>
                  <LockKeyhole size={17} strokeWidth={2.2} className={focused === "password" ? "text-primary" : "text-text3"} />
                  <input type={passwordVisible ? "text" : "password"} autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} onFocus={() => setFocused("password")} onBlur={() => setFocused("")} onKeyDown={(e) => e.key === "Enter" && canSubmit && submit()} placeholder="输入密码" className="ml-2 min-w-0 flex-1 bg-transparent text-[15px] outline-none placeholder:text-text3" />
                  <button type="button" onClick={() => setPasswordVisible((value) => !value)} className="ml-2 text-text3" aria-label={passwordVisible ? "隐藏密码" : "显示密码"}>{passwordVisible ? <EyeOff size={17} /> : <Eye size={17} />}</button>
                </div>
              </label>
              <p className="text-[11px] leading-relaxed text-text3">在「我的 → 账号与安全」设置过密码后可用；审核账号也从这里登录。</p>
            </div>
          )}

          {mode !== "wechat" && <button
            onClick={submit}
            disabled={!canSubmit || submitting || !agreed}
            className="mt-5 flex h-12 w-full items-center justify-center gap-2 rounded-2xl bg-primary text-[15px] font-extrabold text-white shadow-[0_12px_28px_rgba(232,115,74,.32)] transition-all hover:bg-primary-dark active:scale-[.98] disabled:opacity-50 disabled:shadow-none"
          >
            {submitting ? <Loader2 size={18} className="animate-spin" /> : <>登录 <ArrowRight size={18} strokeWidth={2.6} /></>}
          </button>}
        </div>

        <div className="mt-auto pt-6 text-center">
          <div className="flex items-center justify-center gap-1 text-xs leading-relaxed text-text2">
            <label className="flex min-h-11 shrink-0 cursor-pointer items-center gap-2">
              <input type="checkbox" checked={agreed} onChange={(event) => setAgreed(event.target.checked)} className="h-5 w-5 accent-primary" aria-label="同意用户服务协议和隐私政策" />
              我已阅读并同意
            </label>
            <span><a href="/legal?type=terms" onClick={(event) => { event.preventDefault(); setLegalType("terms") }} className="inline-block py-2 text-primary underline underline-offset-4">《用户服务协议》</a>和<a href="/legal?type=privacy" onClick={(event) => { event.preventDefault(); setLegalType("privacy") }} className="inline-block py-2 text-primary underline underline-offset-4">《隐私政策》</a></span>
          </div>
          <a href="/about" className="inline-flex min-h-11 items-center text-xs text-primary underline underline-offset-4">关于我们 · 特色与使用指南</a>
          <div className="mt-2 flex items-center justify-center gap-1.5 text-[11px] text-text3">
            <ShieldCheck size={13} strokeWidth={2.4} />
            你的饮食数据只属于你，AI 也只能读取你授权的部分
          </div>
        </div>
      </div>
      {legalType && <AnimatedBottomSheet label={legalType === "terms" ? "用户服务协议" : "隐私政策"} onClose={() => setLegalType(null)}>
        {({ close }) => <div className="p-5">
          <div className="mb-5 flex items-center justify-between gap-3"><h1 className="text-lg font-bold">{legalType === "terms" ? "用户服务协议" : "隐私政策"}</h1><button type="button" onClick={close} aria-label="关闭协议" className="flex h-11 w-11 items-center justify-center rounded-full bg-bg"><X size={20} /></button></div>
          <LegalContent type={legalType} />
        </div>}
      </AnimatedBottomSheet>}
    </div>
  )
}
