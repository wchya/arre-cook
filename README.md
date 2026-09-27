# arre食谱推荐小助手

> 一款温馨的每日菜单推荐与食谱管理应用，告别"今天吃什么"的纠结。

arre食谱推荐小助手是一个多用户食谱与饮食管理应用，提供邮箱验证码登录、个人数据隔离、口味画像、菜单推荐、用餐记录、周计划和购物清单。每个账号独立管理记录、偏好和私房菜，公共菜谱由管理员维护。

站内 AI 助手可结合当前用户的饮食偏好和历史数据推荐菜品。第三方 Agent 通过用户单独签发的限权令牌接入 MCP、函数调用或 OpenAPI；每个令牌只访问签发者自己的数据，并可随时撤销。项目同时提供原生微信小程序，登录、选菜、个人信息和菜谱编辑等主流程不依赖 WebView。

生产部署步骤见 [部署指南](docs/deployment.md)，Agent 接入见 [接口文档](docs/agent-api.md)，小程序说明见 [微信小程序指南](docs/miniprogram.md)。生产环境需要配置 QQ 邮箱 SMTP、JWT 密钥和 Garage S3 应用密钥。

## 应用截图

以下截图保留自历史版本，当前界面与交互以应用和 [界面规范](docs/ui-guide.md) 为准。

<p align="center">
  <a href="img/home.png"><img src="img/home.png" height="360" alt="首页" /></a>
  <a href="img/dishes.png"><img src="img/dishes.png" height="360" alt="菜品列表" /></a>
  <a href="img/favorites.png"><img src="img/favorites.png" height="360" alt="收藏" /></a>
  <a href="img/tomorrow.png"><img src="img/tomorrow.png" height="360" alt="明天吃什么" /></a>
</p>
<p align="center">
  <a href="img/history.png"><img src="img/history.png" height="360" alt="历史记录" /></a>
  <a href="img/achievements.png"><img src="img/achievements.png" height="360" alt="成就" /></a>
  <a href="img/photo-wall.png"><img src="img/photo-wall.png" height="360" alt="照片墙" /></a>
  <a href="img/more.png"><img src="img/more.png" height="360" alt="更多" /></a>
</p>
<p align="center">
  <a href="img/admin.png"><img src="img/admin.png" height="360" alt="管理后台" /></a>
</p>

- **[使用说明](README_USER.md)** — 如何使用 arre食谱推荐小助手
- **[开发指南](README_DEV.md)** — 如何参与开发和构建
- **[更新日志](CHANGELOG.md)** — 版本更新记录

## 相关链接

* [Linux Do 社区](https://linux.do/)  

## 当前项目状态

> 代码说明更新于 2026-09-27。本轮版本为 v0.10.0；实际部署版本可通过 `/healthz` 查看，版本源文件为 `VERSION`。

应用包含 Web 与原生微信小程序。功能按账号隔离，公共菜谱由管理员维护，家庭菜谱在明确共享后对家庭成员开放。

### 已完成能力

- **账号与多用户**：QQ/Foxmail 邮箱验证码登录、管理员密码兜底登录、微信小程序登录绑定、用户数据独立管理。
- **自动草稿**：新建或编辑菜谱时自动保存，离开后可继续编辑；草稿仅在当前设备、当前账号恢复，成功保存后清除。
- **关于我们**：两端登录页和「我的」提供平台特色、使用步骤、常见问题及协议入口。
- **助手使用保护**：每账号每日最多 20 次，两端共用，管理员可配置 0–20；配合全站预算、并发限制和食谱内容审核。详见 [助手安全与额度](docs/assistant-safety.md)。
- **食谱管理**：公共菜谱、私房菜、多图、抖音/哔哩哔哩视频链接、食材调料、烹饪步骤、难度、烹饪模式、计时、克隆、排序和启用/禁用。
- **智能推荐**：午餐、晚餐、心情、明日菜单、口味画像、偏好过滤、近期去重和推荐理由。
- **饮食管理**：用餐记录、饮食日记、评分、备注、照片墙、7 天/30 天饮食报告、菜系分布和健康建议。
- **计划与采购**：一周菜单、买菜清单、食材合并、分类、勾选、库存标记和家庭采购清单。
- **家庭模式**：邮箱邀请、加入/退出家庭、成员管理、家庭菜谱、家庭菜单和共享买菜清单。个人记录、偏好、饮食日记、AI 对话和 Agent Token 不会因加入家庭而共享。
- **成就与后台**：80+ 成就自动检测、站点设置、菜谱 CRUD、批量操作、用户管理、仪表盘、语录管理和 AI 配置。
- **站内信**：系统更新、新功能、维护、健康、家庭和 Agent 安全通知；支持未读数、单条已读、全部已读和管理员广播。

### Agent 与 AI 接入

站内 AI 助手和外部 Agent 共用同一套按用户隔离的工具体系，支持：

- MCP Streamable HTTP
- OpenAI/DeepSeek Function Calling
- OpenAPI
- Hermes、DSH、DeepSeek Harness、Claude、Cursor、Dify、Coze 等兼容客户端

每个用户可以在「我的 → AI 连接」创建自己的 `nm_` Token，并设置名称、权限范围和有效期。Token 支持查看、修改、撤销和轮换，明文只在创建或轮换时返回一次，数据库只保存 SHA-256 摘要。

Agent 请求始终按照 Token 绑定的 `user_id` 读取数据，不能通过请求参数指定其他用户。`/api/agent/*` 和 `/mcp` 拒绝长期站内登录 JWT，只接受个人 Agent Token 或短期 Agent Session。Hermes 与 DSH 必须分别使用不同用户创建的 Token，禁止多个机器人共享一个 Token。

完整接口说明见 [Agent 接口文档](docs/agent-api.md)。

### 微信小程序

小程序是 `miniprogram/` 下的原生 WXML/WXSS/JavaScript 工程，共 24 个页面，与 Web 共用 Go API 和账号数据。

- 微信已绑定账号可快捷登录；也支持邮箱验证码或已有密码登录。
- 菜谱、用餐记录、家庭、个人信息、站内信等主页面均为原生页面。
- 个人主体使用图片上传时，在微信公众平台配置 `https://cook.arrebyte.top` 的 **request** 与 **uploadFile** 合法域名，并按实际拍照/选图用途提交隐私保护指引。当前主流程无需业务域名。
- 代码上传到微信后台后，仍需体验版真机验收；上传、提交审核、正式发布是不同步骤。

具体域名、隐私说明和上传方式见 [微信小程序指南](docs/miniprogram.md)。

### 生产部署

- **后端**：Go 1.25、Gin、GORM。
- **前端**：React 19、TypeScript、Vite、Tailwind CSS、React Query、Zustand。
- **数据库**：博客服务器 MySQL 8 实例中的独立 `ninimenu` schema，启动时自动迁移；个人表统一使用 `user_id` 作用域。
- **图片存储**：单机 Garage S3，使用独立的 `cook-uploads` bucket 和菜谱站专用密钥。
- **AI 模型**：生产环境读取线上 DSH/CPA 配置，密钥不写入仓库、数据库或日志。
- **反向代理**：博客 Nginx 通过共享 Docker 网络转发到 食谱服务容器。

服务地址：`https://cook.arrebyte.top`。健康检查：`https://cook.arrebyte.top/healthz`。发布时要求容器 healthy，返回预期 `version` 和 `storage=s3`，并验证站内信不重复发送。

生产配置、备份和升级流程见 [部署指南](docs/deployment.md)。单机 Garage 没有跨主机副本，生产环境需要持续执行 MySQL、Garage meta/data 和配置备份，并定期进行恢复演练。

### 测试与交付

```bash
cd backend
go test ./...
go vet ./...
cd ../frontend
npm ci
npm run lint
npm test
npm run build
```

后端覆盖数据隔离、数据库迁移、家庭、Agent 权限、站内信、存储，以及非法 ID 和菜谱更新校验。客户端回归覆盖昵称弹层与菜单生命周期、草稿恢复与账号隔离、历史记录完整分页、登录竞态和富文本菜谱的保存。

本轮页面检查、问题依据和验证边界见 [审查报告](docs/audit-report-arre-cook-2026-09-27.md)。浏览器及组件模拟不能替代 iOS/Android 微信上传、键盘、相机和原生渲染的真机验收。

## 后续事项

- 微信体验版的 iOS/Android 真机验收与隐私指引审核。
- 第三方 Agent 实际客户端联调。
- 定期演练 MySQL、Garage 对象和配置的恢复流程。
- 营养数据等历史设想尚未包含在本轮版本中；当前饮食报告基于用餐记录与菜系信息。

## License

Private — All Rights Reserved
