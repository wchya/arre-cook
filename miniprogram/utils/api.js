const SESSION_KEY = "ninimenu_session"
let redirecting = false

function queryString(params) {
  if (!params) return ""
  const pairs = []
  Object.keys(params).forEach((key) => {
    const value = params[key]
    if (value === undefined || value === null || value === "") return
    pairs.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
  })
  return pairs.length ? `?${pairs.join("&")}` : ""
}

function request(method, path, data) {
  const app = getApp()
  const isGet = method === "GET"
  const token = wx.getStorageSync(SESSION_KEY) || ""
  const url = `${app.globalData.apiBase}${path}${isGet ? queryString(data) : ""}`

  return new Promise((resolve, reject) => {
    wx.request({
      url,
      method,
      data: isGet ? undefined : (data || {}),
      timeout: 20000,
      header: {
        "content-type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      success(response) {
        const body = response.data || {}
        if (response.statusCode >= 200 && response.statusCode < 300) redirecting = false
        if (response.statusCode === 401) {
          try { wx.removeStorageSync(SESSION_KEY) } catch (_) { /* ignore */ }
          if (!redirecting) {
            redirecting = true
            app.globalData.sessionRestoreAttempted = true
            wx.reLaunch({ url: "/pages/login/login?reason=expired" })
          }
          reject(new Error(body.message || "登录已过期，请重新登录"))
          return
        }
        if (response.statusCode < 200 || response.statusCode >= 300 || body.code !== 0) {
          reject(new Error(body.message || "服务暂时不可用"))
          return
        }
        resolve(body.data)
      },
      fail() { reject(new Error("网络连接失败，请稍后重试")) },
    })
  })
}

module.exports = {
  get: (path, params) => request("GET", path, params),
  post: (path, data) => request("POST", path, data),
  put: (path, data) => request("PUT", path, data),
  patch: (path, data) => request("PATCH", path, data),
  delete: (path, data) => request("DELETE", path, data),
  queryString,
}
