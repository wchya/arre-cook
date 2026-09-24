// 微信小程序 web-view 环境适配。
// 小程序壳（miniprogram/）原生完成登录后，把令牌放在 URL 片段里打开 H5：
//   https://cook.arrebyte.top/?from=mp#token=<jwt>
// 片段不会随请求发往服务器、也不进 Referer；H5 读取后立即从地址栏抹掉。

interface WxMiniProgram {
  navigateTo(opts: { url: string }): void
  navigateBack(opts?: { delta?: number }): void
  reLaunch(opts: { url: string }): void
  postMessage(opts: { data: unknown }): void
  getEnv(cb: (res: { miniprogram: boolean }) => void): void
}

declare global {
  interface Window {
    __wxjs_environment?: string
    wx?: { miniProgram?: WxMiniProgram }
  }
}

const JSSDK_URL = "https://res.wx.qq.com/open/js/jweixin-1.6.0.js"
const MP_FLAG_KEY = "ninimenu_from_mp"

export function isWeChat(): boolean {
  return /micromessenger/i.test(navigator.userAgent)
}

export function isMiniProgram(): boolean {
  if (typeof window === "undefined") return false
  if (window.__wxjs_environment === "miniprogram" || /miniprogram/i.test(navigator.userAgent)) return true
  try {
    return sessionStorage.getItem(MP_FLAG_KEY) === "1"
  } catch {
    return false
  }
}

let sdkPromise: Promise<WxMiniProgram | null> | null = null

// 按需加载 JSSDK（只在小程序 web-view 里需要）
export function loadMiniProgramSDK(): Promise<WxMiniProgram | null> {
  if (!isMiniProgram()) return Promise.resolve(null)
  if (window.wx?.miniProgram) return Promise.resolve(window.wx.miniProgram)
  if (sdkPromise) return sdkPromise
  sdkPromise = new Promise((resolve) => {
    const script = document.createElement("script")
    script.src = JSSDK_URL
    script.async = true
    script.onload = () => resolve(window.wx?.miniProgram ?? null)
    script.onerror = () => resolve(null)
    document.head.appendChild(script)
  })
  return sdkPromise
}

// 读取小程序带来的令牌（URL 片段 #token=…），读完抹掉，返回令牌或 null
export function consumeTokenFromURL(): string | null {
  const url = new URL(window.location.href)
  const hash = new URLSearchParams(url.hash.replace(/^#/, ""))
  const token = hash.get("token")
  const fromMP = url.searchParams.get("from") === "mp"
  if (fromMP) {
    try {
      sessionStorage.setItem(MP_FLAG_KEY, "1")
    } catch {
      /* 隐私模式忽略 */
    }
  }
  if (!token && !fromMP) return null
  url.hash = ""
  url.searchParams.delete("from")
  window.history.replaceState(window.history.state, "", url.pathname + url.search)
  return token
}

// 登录失效或主动退出时，回到小程序原生登录页
export async function backToMiniProgramLogin(reason: "expired" | "logout") {
  const mp = await loadMiniProgramSDK()
  if (mp) {
    mp.postMessage({ data: { type: "logout" } })
    mp.reLaunch({ url: `/pages/login/login?reason=${reason}` })
    return true
  }
  return false
}

// 告诉小程序当前页面的分享信息（小程序在用户点分享时读取最近一条消息）
export async function setMiniProgramShare(title: string, path: string) {
  const mp = await loadMiniProgramSDK()
  mp?.postMessage({ data: { type: "share", title, path: path.startsWith("/") && !path.startsWith("//") ? path : "/" } })
}
