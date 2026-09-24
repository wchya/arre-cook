# 微信小程序

## 工程配置

小程序工程在 `miniprogram/`，导入微信开发者工具时选择这个目录。项目 AppID 已配置；服务端 AppSecret 只放在根目录被 Git 忽略的 `.env` 中，通过 `WECHAT_APPID` 和 `WECHAT_SECRET` 传给后端。

默认 H5 和 API 地址为 `https://cook.arrebyte.top`，修改地址时同时调整 `miniprogram/app.js` 的 `apiBase`、`webBase`。小程序原生登录页先尝试微信快捷登录；微信未绑定时可以用 QQ/Foxmail 邮箱验证码登录，并在同一账号上绑定新获取的 `wx.login` code。服务端从不把 AppSecret 下发给小程序。

WebView 令牌经 URL fragment 交给 H5，读取后立即清除；令牌不会放进 HTTP 请求路径或 Referer。H5 退出或令牌过期后会返回原生登录页。分享路径限制在本站内。

## 微信公众平台

发布前配置以下平台信息：

- 在小程序后台将 `cook.arrebyte.top` 添加为业务域名和 request 合法域名；WebView 页面使用 HTTPS。
- 若启用微信快捷登录，服务器配置正确的 `WECHAT_APPID` 与 `WECHAT_SECRET`。
- 在微信开发者工具中确认工程 AppID 与服务器 AppID 一致，并完成主体认证、域名校验和隐私保护指引。
- 检查 WebView 路由、H5 登录、邮箱验证码、退出、分享，以及 iOS/Android 真机图片上传。

本地调试可以在开发者工具中按项目需要关闭域名校验，但体验版和正式版必须使用已备案并校验的 HTTPS 域名。`touristappid` 不能用于体验版或正式版。

## 上传代码

`docs/private.<AppID>.key` 是微信小程序代码上传 RSA 私钥，已从 Git 忽略并收紧为当前用户可读写。保留该文件在本机的 `docs/` 目录，不要复制到小程序工程、前端资源、镜像或环境变量中。它用于开发者工具/微信 CI 上传，不参与用户登录。

日常开发可直接用微信开发者工具预览和上传。若部署自动上传流水线，应由 CI 将这把私钥作为受保护文件挂载，并通过微信官方 `miniprogram-ci` SDK 上传；上传动作会把版本提交到微信后台，本地实现不自动触发发布。
