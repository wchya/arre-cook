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

function token() {
  try { return wx.getStorageSync(SESSION_KEY) || "" } catch (_) { return "" }
}

function expireSession(app) {
  try { wx.removeStorageSync(SESSION_KEY) } catch (_) { /* ignore */ }
  app.globalData.user = null
  if (redirecting) return
  redirecting = true
  app.globalData.sessionRestoreAttempted = true
  wx.reLaunch({ url: "/pages/login/login?reason=expired" })
}

function request(method, path, data) {
  const app = getApp()
  const isGet = method === "GET"
  // 登录接口不带旧令牌：密码错误返回 401 时不能被当成“登录过期”。
  const auth = path.indexOf("/auth/") === 0 ? "" : token()
  const url = `${app.globalData.apiBase}${path}${isGet ? queryString(data) : ""}`

  return new Promise((resolve, reject) => {
    wx.request({
      url,
      method,
      data: isGet ? undefined : (data || {}),
      timeout: 20000,
      header: {
        "content-type": "application/json",
        ...(auth ? { Authorization: `Bearer ${auth}` } : {}),
      },
      success(response) {
        const body = response.data || {}
        if (response.statusCode >= 200 && response.statusCode < 300) redirecting = false
        if (response.statusCode === 401 && auth) {
          expireSession(app)
          reject(new Error(body.message || "登录已过期，请重新登录"))
          return
        }
        if (response.statusCode < 200 || response.statusCode >= 300 || body.code !== 0) {
          const error = new Error(body.message || "服务暂时不可用")
          error.status = response.statusCode
          error.data = body.data
          reject(error)
          return
        }
        resolve(body.data)
      },
      fail() { reject(new Error("网络连接失败，请稍后重试")) },
    })
  })
}

// 上传图片：返回服务端给出的原始地址（/uploads/... 或对象存储地址），提交给接口时必须用原始地址。
function upload(filePath) {
  const app = getApp()
  return new Promise((resolve, reject) => {
    wx.uploadFile({
      url: `${app.globalData.apiBase}/upload/image`,
      filePath,
      name: "image",
      timeout: 60000,
      header: { Authorization: `Bearer ${token()}` },
      success(response) {
        let body = {}
        try { body = JSON.parse(response.data || "{}") } catch (_) { body = {} }
        if (response.statusCode === 401) {
          expireSession(app)
          reject(new Error("登录已过期，请重新登录"))
          return
        }
        if (response.statusCode < 200 || response.statusCode >= 300 || body.code !== 0 || !body.data || !body.data.url) {
          reject(new Error(body.message || "图片上传失败"))
          return
        }
        resolve(body.data.url)
      },
      fail() { reject(new Error("图片上传失败，请检查网络")) },
    })
  })
}

// 行为埋点：浏览、采纳、拒绝推荐等，失败静默，不影响主流程。
function behavior(event) {
  return request("POST", "/behavior", event).catch(() => null)
}

module.exports = {
  SESSION_KEY,
  token,
  get: (path, params) => request("GET", path, params),
  post: (path, data) => request("POST", path, data),
  put: (path, data) => request("PUT", path, data),
  patch: (path, data) => request("PATCH", path, data),
  delete: (path, data) => request("DELETE", path, data),
  upload,
  behavior,
  queryString,
}
