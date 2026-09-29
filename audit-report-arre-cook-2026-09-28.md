# Fuck My Shit Mountain Audit Report

**Project:** arre-cook（arre食谱推荐小助手 / 模块名 ninimenu）
**Audit mode:** full（重点：安全、性能、可扩展架构、极端场景、UI 组件交互）
**Date:** 2026-09-28
**Reviewer:** Claude Opus 5.5（claude-opus-5-5，1M context）

> 审计对象为工作区当前磁盘状态（含未提交改动）。本次审计期间 Bash/子代理工具间歇性不可用，全部结论由主审逐文件阅读与 grep 取证；`go vet ./...` 通过、`go test ./...` 全部通过（本机 SQLite，MySQL 用例未提供 socket 时跳过）。仓库内 `docs/audit-report-arre-cook-2026-09-28.md` 与 `docs/audit-remediation-2026-09-28.md` 所列 F01–F19 已修复项不重复报告，本报告只列出在其修复之后仍存在或新发现的问题。

---

## 1. Executive Summary

整体来看，这是一个**防御意识明显高于同类个人项目**的代码库：密码使用 PBKDF2-SHA512 21 万次迭代并常量时间比较；改密使用 CAS 防止旧令牌复活；Agent/MCP 工具全部带 scope 且统一走 `agent.Invoke` 审计；视频/链接预览的 SSRF 防护在 dial 阶段校验 IP、禁用重定向与代理；上传按内容嗅探、先读 DecodeConfig 限制像素；数据库限流窗口、助手租约、配额均为 SQL 条件原子更新；Web 端还有"每标签页会话世代"防止跨账号串号。前一轮审计的 19 项问题已系统修复，测试 273 项（含 race）通过。

但在**极端场景**下，本轮仍发现两类系统性风险：
其一，**"为了保护小主机而设的硬上限"本身成了单点**。进程内限流表满 10000 个 key 后对所有新 IP 一律拒绝（未鉴权即可触发）；全站 API 在鉴权前只有 16 个并发槽；图片回源 8 个并发槽；图片上传 1 个槽且与视频 ASR 互斥。这些上限均是 fail-closed、全站共享、按进程计数，在流量尖峰、IPv6 地址轮换、慢数据库或多副本部署时会从"保护"变成"整站 429"。
其二，**UI 数据截断与静默失败**：助手会话超过 200 条后界面只显示最早的 200 条；选菜/家庭菜单选择器只拉前 100 道菜并在客户端搜索，超过 100 道即"搜不到"；批量改分类不校验、写库失败仍提示成功；小程序在视频提炼中选图会触发 onHide 静默取消并已扣次数。

另外有一项配置层高风险：生产启动检查只拦截内置默认值，`.env.example` 中的占位密钥（`replace-with-...`）可以原样通过，一旦运维漏改，JWT 密钥与初始管理员密码即为公开值。

整体评级 **B（6.6）**：安全基础扎实、修复纪律好；需要优先处理"硬上限的 fail-closed 语义"和"占位密钥 fail-fast"，再补齐 CI 与 UI 截断问题。

### Score Dashboard

```
Security        ███████░░░  7.0  A   基础扎实；限流表满即全站拒绝、占位密钥可通过启动检查、PAT 不随改密失效
Stability       ███████░░░  7.0  A   超时/预算完善；启动时单事务全员发通知、多处写库吞错仍返回成功
Performance     ██████░░░░  6.0  B   全站 16 并发槽/8 回源槽/1 上传槽硬上限；多张表无清理；批量删除 N+1
Testing         ███████░░░  6.5  B   273 项测试含 race；无 CI，MySQL 并发语义默认跳过，小程序零自动化
Maintainability ███████░░░  6.5  B   Tomorrow.tsx 1028 行、plan_service 825 行；Web 与小程序双份业务逻辑
Design          ███████░░░  7.0  A   原子化/锁顺序设计好；配置非 fail-fast，分页参数静默回退
Release         ██████░░░░  6.0  B   容器加固到位；无 CI 流水线，发布/漏洞扫描依赖手工
─────────────────────────────────────
Overall         ███████░░░  6.6  B
```

Each dimension scored 0.0–10.0. **Higher = better (10 = clean, 0 = shit mountain).** Scores are judgment-based, not formula-based.

### Finding Statistics

| Severity | Count | Confirmed | Suspected |
|----------|-------|-----------|-----------|
| Critical | 0 | 0 | 0 |
| High | 2 | 2 | 0 |
| Medium | 17 | 14 | 3 |
| Low | 8 | 7 | 1 |
| Info | 2 | 2 | 0 |
| **Total** | **29** | **25** | **4** |

## 2. Project Map

**组件与职责**

| 组件 | 位置 | 职责 |
|---|---|---|
| API 服务 | `backend/cmd/server` + `internal/*` | gin + GORM；用户/家庭/菜谱/记录/周菜单/购物/成就/通知/AI 助手/视频提炼/Agent REST/MCP；同进程托管 SPA 与 `/uploads` 回源 |
| 迁移工具 | `backend/cmd/dbmigrate` | 独立迁移（`DB_MIGRATE_ONLY`、`DB_AUTO_MIGRATE=false` 的副本） |
| Web 前端 | `frontend/`（React 19 + react-query + zustand + Vite） | 用户端与管理后台；构建产物放入 `backend/static` |
| 微信小程序 | `miniprogram/`（原生，25 个页面，自定义 tabbar/导航栏） | 原生登录、聊天流式（`enableChunked`）、上传 |
| 本地 ASR | `backend/video_asr`（Python SenseVoice，seccomp 沙箱子进程） | 视频语音转写，与图片解码共享一个"重任务"槽 |

**入口与初始化顺序**（`cmd/server/main.go`）：`config.Load` → `database.Init`（MySQL advisory lock → AutoMigrate → seed → 引导管理员 → 多用户迁移 → 上传账本）→ `AnnounceDeploymentWithNotes`（**在监听端口之前**）→ gin 中间件链 → `StartAchievementWorkers` → `housekeeping` 每小时 → `ListenAndServe`（无 WriteTimeout，SSE 需要）→ SIGTERM 后 100s Shutdown → `resourcebudget.Drain`。

**请求生命周期**（`routes.Setup`）：`SecurityHeaders → RequestBudget（每 IP 360/分、全站 16 并发槽、按路径 15/30/90s 超时、64/256 KiB body、JSON 结构上限）→ AIIngress（AI/认证路径每 IP 240/分、Origin 校验、再读 body）→ CORS → Logger → 路由组（UserAuth / AdminOnly / AgentAuth+RequireScope）`。

**状态归属**：持久状态全部在 MySQL（生产）/ SQLite（默认/测试）；共享限流窗口 `request_windows`、助手租约 `assistant_leases`、日配额 `assistant_usages`、任务键 `task_claims` 均为数据库行。**进程内状态**：`windowLimiter` 多个实例、`apiSlots(16)`、`passwordSlots(2)`、`largeReadSlots(2)`、`imageUploadSlot(1)+waiters(4)`、`resourcebudget.slot(1)`、`storage.originSlots(8)`、成就 worker 队列、视频缓存（128 项/20 MiB）。

**外部接口**：DeepSeek/CPA（LLM，75s 超时，禁重定向）、SMTP（QQ，20s）、微信 code2session（8s）、Garage S3、抖音/B 站（`PublicHTTPClient`，dial 级 SSRF 防护）、远程 ASR（可选）。

**安全边界**：用户 JWT（HS256，30 天，TokenVersion 撤销）、Agent 会话 JWT（2h，带 scope，随 TokenVersion 撤销）、PAT（`nm_` 前缀，SHA-256 存储，**不随 TokenVersion 撤销**）；家庭多租户通过 `VisibleDishes` / `LockFamilyForUser` 统一锁顺序。

**部署拓扑**：单 Docker Compose 服务，`127.0.0.1:9925` → 宿主 Nginx；MySQL 与 Garage 位于博客站 Docker 网络；非 root、`cap_drop: ALL`、1 GiB/2 CPU、`GOMEMLIMIT=192MiB`；`.env.example` 提到"API replicas"，即存在多副本的设计意图。

**风险集中区**：`middleware/`（全站硬上限）、`storage/store.go`（回源）、`handlers/upload.go` + `resourcebudget`、`handlers/dish.go` 批量接口、`services/notification_service.go` 启动路径、各客户端"取第一页 100 条"的选择器。

## 3. Top Risks

| # | Finding | Severity | 一句话摘要 |
|---|---|---|---|
| F01 | 进程内限流表满 10000 key 后拒绝所有新 IP | High | 未鉴权攻击者用 1 万个不同 IP（IPv6 /64 轻易做到）即可让所有新访客/登录 429，认证限流窗口长达 10 分钟 |
| F02 | 占位密钥可通过生产启动检查 | High | `JWT_SECRET=replace-with-...` 长度 ≥16 且非默认值，服务照常启动，JWT 可被任何读过仓库的人伪造 |
| F05 | 全站仅 16 个鉴权前 API 并发槽 | Medium | 慢 DB、慢上传体或 90s 长连接占满后全站 429，而 `/healthz` 仍报 UP |
| F07 | 图片上传全局 1 槽且与 ASR 互斥 | Medium | 任何人做视频语音转写的 ≤50s 内，全站上传立即失败 |
| F06 | 图片回源全站 8 并发 | Medium | 照片墙/菜谱列表首屏几十张图并发加载即部分 429 破图 |
| F13 | 会话超过 200 条只显示最早 200 条 | Medium | 最新回复"消失"，界面与模型记忆不一致 |
| F14 | 选择器只拉前 100 道菜、客户端搜索 | Medium | 私房菜+公共菜超过 100 后，明日菜单/家庭菜单里搜不到菜 |
| F12 | 批量改分类不校验且吞错 | Medium | 超长分类撑破筛选栏；MySQL 下超 191 字写入失败却提示成功 |
| F03 | PAT 不随改密 / 退出所有设备失效 | Medium | 账号被盗后改密无法踢掉攻击者创建的长期 Agent 令牌 |
| F08 | 启动路径上单事务给全体用户写通知 | Medium | 用户量大时启动被阻塞在监听之前，失败后每次重启重试 |
| F15 | 小程序视频提炼中选图会静默取消并已计次 | Medium | 选封面触发 onHide → 取消 SSE，用户次数被扣且无提示 |
| F16 | Web 记录页多图上传无上传态 | Medium | 上传中点"保存评价"丢照片，连点"+"可超过 9 张 |
| F17 | 多处写库忽略错误仍返回成功 | Medium | DB 抖动时 UI 显示已保存，刷新后回退 |
| F09 | 通知/行为/聊天等表无清理 | Medium | 每次发版 × 全体用户写通知，表无界增长 |
| F18 | 无 CI 流水线 | Medium | 测试、vet、govulncheck、race 全靠人工 |

## 4. Detailed Findings

### Finding: F01 进程内限流表满 10000 key 后对所有新 IP fail-closed，可被未鉴权请求触发全站拒绝

- Severity: High
- Confidence: Medium
- Category: Security
- Status: Confirmed
- Affected area: `internal/middleware`（`windowLimiter`、`RequestBudget`、`AuthRateLimit`、`AIIngress`）
- Evidence:
  - File: `backend/internal/middleware/middleware.go:234-267`
  - Function / Module: `(*windowLimiter).AllowN`
  - Relevant behavior: 新 key（`b == nil`）且 `len(l.buckets) >= 10000` 时，只清理已过期桶；清理后仍 ≥10000 就 `return false`。
  - `backend/internal/middleware/request_budget.go:22,36-39`：`apiLimiter.Allow(c.ClientIP(), 360)` 作用于**所有** `/api/*` 与 `/mcp`，位于鉴权之前。
  - `middleware.go:271,276-286`：`authLimiter` 窗口 **10 分钟**，key 为 `auth:<IP>`；`ai_guard.go:20,34` 同样模式。
- Problem: 限流器的"容量上限"语义是拒绝，而不是淘汰旧 key 或退化为共享 DB 窗口。只要 1 分钟内出现 10000 个不同客户端 IP，第 10001 个及之后的**所有新 IP** 对全部 API 都拿到 429；认证限流表的窗口是 10 分钟，只需约 17 个新 IP/秒就能让所有新用户在 10 分钟内无法登录。已有桶的老用户在自己的桶过期（1 分钟）后也变成"新 key"被拒。
- Why it matters: 这是一个无需账号、无需高带宽的整站可用性开关；也是真实流量增长时的"悬崖"——站点变热时不是变慢，而是突然整体拒绝。
- Realistic failure scenario: 攻击者持有一个 IPv6 /64（或小型代理池），以每秒 170 个不同源地址请求 `GET /api/app-info`（公开接口）。约 60 秒后 `apiLimiter` 满载，此后所有真实用户打开小程序/网页的首个请求返回 `42900 请求过于频繁`；攻击者持续发送即可维持。`/healthz` 不经过该中间件，容器健康检查仍为 UP，不会触发任何告警或重启。
- Minimal fix: 容量满时改为淘汰最早的桶（或按 `start` 最老的若干个），而不是拒绝；IPv6 客户端按 /64 归一化 key；对 `apiLimiter` 在容量满时 fail-open 并依赖 `apiSlots` 兜底。
- Better long-term fix: 把按 IP 的粗限流下沉到 Nginx `limit_req`/`limit_conn`（已有反代），应用层只保留按账号/令牌的 DB 窗口；为限流拒绝率增加指标与告警。
- Regression test suggestion: 单元测试向 `newWindowLimiter(time.Minute)` 注入 10000 个不同 key 后，断言第 10001 个新 key 仍能 `Allow`；再断言相同 key 超过 limit 时依然被拒。
- Estimated effort: 2–4 小时

### Finding: F02 生产启动检查只拦截内置默认值，`.env.example` 占位密钥可原样通过

- Severity: High
- Confidence: High
- Category: Security
- Status: Confirmed
- Affected area: `internal/config`、部署配置
- Evidence:
  - File: `backend/internal/config/config.go:92,97,105,183-190`
  - Function / Module: `config.Load`
  - Relevant behavior: 生产环境仅当 `JWT_SECRET == "ninimenu-secret-key"` 或长度 <16 时**打印警告**；`ADMIN_PASSWORD` 仅与 `"nini123"` 比较并打印警告，均不终止启动。
  - `.env.example:16-17`：`ADMIN_PASSWORD=replace-with-a-long-random-password`、`JWT_SECRET=replace-with-at-least-32-random-bytes`（35 字符，通过长度检查，且不会触发任何警告）。
  - `docker-compose.yml:47-48`：`${JWT_SECRET:?...}` 只拒绝空值。
- Problem: 按文档"复制 .env.example 为 .env"后忘记替换，服务会静默使用公开的 JWT 签名密钥和公开的初始管理员密码。`JWT_SECRET` 同时参与邮箱验证码哈希（`auth_service.go:65-68`）和家庭邀请哈希（`family_service.go:193-196`）。
- Why it matters: 一旦发生，攻击者可以离线签发任意 `sub`、`typ=user` 的 JWT（`TokenVersion` 为 1 的账号直接可用），或用公开密码登录管理员，属于完全接管。
- Realistic failure scenario: 新机器部署或灾备重建时，运维复制 `.env.example` 只填了 MySQL/S3。容器正常启动且无任何警告；攻击者用 `replace-with-at-least-32-random-bytes` 签发 `{"typ":"user","sub":"1","ver":1,"iss":"ninimenu"}` 即以管理员身份调用 `/api/admin/settings` 读取并修改 LLM 配置。
- Minimal fix: 生产环境 `log.Fatal`：JWT_SECRET 为默认值、长度 <32、或以 `replace-with` 开头；首次引导管理员时 ADMIN_PASSWORD 为 `nini123`、以 `replace-with` 开头或长度 <12 时拒绝创建。
- Better long-term fix: 引入统一的 `config.Validate()`（fail-fast），覆盖 JWT、ADMIN、MySQL、S3 密钥占位符；支持 Docker secrets 文件形式注入。
- Regression test suggestion: `APP_ENV=production JWT_SECRET=replace-with-at-least-32-random-bytes` 调用 `Validate()` 断言返回错误；默认值与 31 字符随机串同样报错。
- Estimated effort: 1 小时

### Finding: F03 个人访问令牌（PAT）不随改密、退出所有设备失效

- Severity: Medium
- Confidence: High
- Category: Security
- Status: Confirmed
- Affected area: 认证 / Agent 令牌
- Evidence:
  - File: `backend/internal/middleware/middleware.go:87-112`（`resolvePAT` 只校验 hash、撤销、过期、账号禁用，不校验 `TokenVersion`）
  - File: `backend/internal/handlers/auth.go:194-245`（`ChangePassword` / `LogoutAll` 只递增 `token_version`；注释明确"智能体令牌不受影响，需单独撤销"）
  - File: `backend/internal/handlers/agent.go:646`（有效期 0–3650 天，0 为永不过期）
- Problem: 账号被盗场景下，攻击者只需在会话有效期内调用 `POST /api/me/agent-tokens` 创建一个全 scope、永不过期的 PAT。受害者随后改密、"退出所有设备"都无法使该令牌失效，必须主动进入"AI 连接"逐个撤销，而用户通常不知道这个入口。
- Why it matters: "改密 = 踢掉所有人"是用户的普遍心智模型；当前实现让最常用的应急操作失效。
- Realistic failure scenario: 用户在公共电脑登录未退出 → 他人创建名为"Hermes"的 PAT → 用户发现后改密 → 他人继续通过 `/mcp` 读取饮食记录、偏好与过敏信息。
- Minimal fix: `ChangePassword` 与 `LogoutAll` 同事务撤销该用户全部 PAT（或提供勾选项并默认勾选）；前端改密成功后提示"已同时撤销 N 个 AI 连接"。
- Better long-term fix: PAT 记录创建时的 `token_version`，`resolvePAT` 校验一致；PAT 有效期上限收紧到 365 天。
- Regression test suggestion: 创建 PAT → 改密 → 用该 PAT 调 `/api/agent/me` 断言 401；`logout-all` 同样断言。
- Estimated effort: 2 小时

### Finding: F04 邮箱验证码的错误次数"先读后增"，并发猜测可突破 5 次上限

- Severity: Medium
- Confidence: High
- Category: Security
- Status: Confirmed
- Affected area: `services/auth_service.go` 邮箱登录
- Evidence:
  - File: `backend/internal/services/auth_service.go:163-184`
  - Function / Module: `VerifyEmailCode`
  - Relevant behavior: 先 `First(&rec)` 读 `attempts`，`>=5` 才拒绝；比较失败后才 `attempts + 1`。并发请求都读到 `attempts=0`，全部进入比较。
- Problem: 5 次上限不是原子的。实际并发度被全站 `apiSlots(16)` 与每 IP 30 次/10 分钟限流约束，因此单个验证码可被猜约 16–20 次而不是 5 次；影响有限但属于确定的逻辑缺陷，且管理员邮箱（`ADMIN_EMAIL`）同样走此路径。
- Why it matters: 邮箱验证码是主要登录方式，也是自动注册入口；计数器是抵御在线暴力破解的核心约束。
- Realistic failure scenario: 攻击者为目标邮箱触发发送验证码，随后用 16 个连接同时提交 16 个不同 6 位码，重复 6 次/小时（每邮箱发送上限）→ 每小时约 100+ 次猜测而非 30 次；使用多 IP 可绕开每 IP 限流。
- Minimal fix: 比较前先执行 `UPDATE email_codes SET attempts = attempts + 1 WHERE id = ? AND attempts < 5 AND consumed_at IS NULL`，`RowsAffected == 0` 直接返回 `ErrCodeTooMany`，成功后再比较哈希。
- Better long-term fix: 增加每邮箱的全局失败窗口（跨验证码），多次失败后要求冷却。
- Regression test suggestion: 起 20 个 goroutine 并发用错误码调用 `VerifyEmailCode`，断言最终 `attempts == 5` 且至多 5 次进入比较（可通过计数钩子或检查 DB 值）。
- Estimated effort: 1 小时

### Finding: F05 全站只有 16 个鉴权前 API 并发槽，慢依赖或长连接即可整站 429，且健康检查无感知

- Severity: Medium
- Confidence: Medium
- Category: Performance
- Status: Confirmed
- Affected area: `middleware/request_budget.go`
- Evidence:
  - File: `backend/internal/middleware/request_budget.go:18,39-46,55-66,69-78`
  - Function / Module: `RequestBudget`
  - Relevant behavior: `apiSlots = make(chan struct{}, 16)`，在鉴权与读 body **之前**获取，`defer` 到 handler 结束才释放；SSE（助手、视频提炼）与 `/mcp` 的超时为 90s；请求体 `io.ReadAll` 在持槽期间执行。
  - `backend/internal/routes/routes.go:28,241-251`：`/healthz` 不经过该预算，只 ping DB。
- Problem: 16 是整个进程所有用户共享的硬上限，且拒绝时返回"请求过于频繁"。任何让单个请求变慢的因素（MySQL 锁等待接近 15s、S3 慢、4 路助手 SSE + 若干 MCP 调用、反代未缓冲时的慢速请求体）都会把剩余请求全部挤成 429，前端显示成"频繁"而不是"服务繁忙"。
- Why it matters: 把"保护小主机"变成了"放大任何抖动"的级联故障点；多副本时容量随副本数线性变化，与共享 DB 窗口的语义不一致。
- Realistic failure scenario: 夜间 MySQL 备份导致部分查询 10s+；16 个在途请求等待 → 之后 10 秒内所有用户的首页、菜谱列表、登录都返回 42900；`/healthz` 返回 UP，Compose 健康检查与外部探活都不会报警。若 Nginx 关闭了 `proxy_request_buffering`（部署文档未规定），单个 IP 发起 16 个慢速 POST（ReadTimeout 30s 内持续滴流）即可重复制造同样效果。
- Minimal fix: 把 SSE/MCP 等长连接从 `apiSlots` 中剥离到单独的槽；槽满时先短暂排队（如 200ms）再拒绝；错误文案区分"服务繁忙"；`/healthz` 增加"最近 1 分钟预算拒绝率"字段。
- Better long-term fix: 以 Nginx `limit_conn` 与 `proxy_request_buffering on` 作为第一道闸（写进部署文档），应用层并发槽按路由类别分池并暴露指标。
- Regression test suggestion: 用 16 个阻塞的 handler 占满槽，断言第 17 个 `/api/app-info` 在排队窗口内得到服务或返回 503 而非 42900；断言 `/api/assistant/chat` 的长连接不占用普通池。
- Estimated effort: 半天

### Finding: F06 `/uploads` 回源全站 8 并发，首屏多图并发加载即部分 429 破图

- Severity: Medium
- Confidence: Medium
- Category: Performance
- Status: Suspected
- Affected area: `storage/store.go` 图片回源；照片墙、菜谱列表、首页
- Evidence:
  - File: `backend/internal/storage/store.go:145,163-175,182-213`
  - Function / Module: `ServeUploads`
  - Relevant behavior: 本地不存在的对象经 S3 回源，`originSlots` 容量 8，满则立即 `429 image service busy`；该路由公开、无鉴权、不经过 `RequestBudget`。
  - `.env.example:53`、`docker-compose.yml:65`：`S3_PUBLIC_URL=/uploads/`（默认所有用户图片都走应用回源）。
- Problem: 用户上传的图片全部经应用回源。浏览器 HTTP/2 下照片墙、菜谱列表会同时发起数十个图片请求，冷缓存时超过 8 个即有图片直接 429；客户端 `<img>`/`<image>` 没有重试，表现为随机破图。是否触发取决于 Nginx/CDN 是否缓存 `/uploads/`（部署文档未说明），故标记为 Suspected。
- Why it matters: 这是正常使用即可触发的 UI 错乱（随机缺图），同时也给未鉴权请求留下了挤占图片服务的入口。
- Realistic failure scenario: 家庭成员首次打开"照片墙"（`frontend/src/pages/PhotoWall.tsx`），页面同时请求 30 张 S3 图片 → 22 张 429 → 显示占位图；刷新后因部分已被浏览器缓存才逐渐完整。
- Minimal fix: 槽满时短暂排队（如 2s）而非立即 429；为 `/uploads/` 在 Nginx 配置 `proxy_cache`（对象键不可变，天然适合长缓存），并写进部署文档。
- Better long-term fix: `S3_PUBLIC_URL` 指向 CDN/反代直连 Garage 的只读域名，应用不再承担图片流量。
- Regression test suggestion: httptest 启动假 S3（每次响应延迟 200ms），并发 30 个 `GET /uploads/...`，断言全部 200（排队）且峰值并发 ≤8。
- Estimated effort: 2–4 小时（代码）+ 反代配置

### Finding: F07 图片上传全站 1 个槽，且与本地 ASR 共享"重任务"槽，转写期间全站上传立即失败

- Severity: Medium
- Confidence: High
- Category: Performance
- Status: Confirmed
- Affected area: `handlers/upload.go`、`resourcebudget`
- Evidence:
  - File: `backend/internal/handlers/upload.go:41-76,135-150`
  - Relevant behavior: `imageUploadSlot` 容量 1、`imageUploadWaiters` 4、最多等 5s；取得后再调用 `resourcebudget.Acquire`，该函数**非阻塞**，被占用则立即返回 `正在处理图片或视频，请稍后重试`。
  - File: `backend/internal/resourcebudget/heavy.go:11-26`（全局 1 槽）；`backend/internal/video/local_asr.go:45,175`（ASR 持槽最长 50s）。
  - 持槽期间还包含读取 multipart 请求体（受 ReadTimeout 30s 约束）与 `storage.Save` 的 S3 网络写入。
- Problem: 全站同一时刻只能处理一张图片；任何一个用户的视频语音转写都会让全站上传在最长 50 秒内全部失败，而不是排队。
- Why it matters: 家庭共享场景下多人同时记餐传图、菜谱编辑多图上传都是常规操作；错误对用户不可解释（"正在处理视频"而自己并没有在处理视频）。
- Realistic failure scenario: 成员 A 在小程序里对一个 3 分钟抖音视频做提炼（进入本地 ASR）；同一时间成员 B、C 在记录页各选 5 张照片上传 → 全部失败，toast"正在处理图片或视频"；B 重试 3 次后放弃。
- Minimal fix: `resourcebudget` 提供带超时的阻塞获取（如等待 10s），上传侧使用；上传读取请求体阶段不持有重任务槽（仅解码/压缩阶段持有）；S3 写入移出重任务槽。
- Better long-term fix: 按内存预算（字节数）而不是槽位准入；ASR 独立进程/容器，与 API 的图片解码不共享预算。
- Regression test suggestion: 持有 `resourcebudget` 槽的同时并发上传 2 张图，断言在释放后两张都在等待窗口内成功，而不是立即 429。
- Estimated effort: 半天

### Finding: F08 发布通知在启动路径、监听端口之前，以单个事务给全体用户写入

- Severity: Medium
- Confidence: Medium
- Category: Stability
- Status: Confirmed
- Affected area: `services/notification_service.go`、`cmd/server/main.go`
- Evidence:
  - File: `backend/cmd/server/main.go:39-41`（`AnnounceDeploymentWithNotes` 在 `ListenAndServe` 之前同步执行）
  - File: `backend/internal/services/notification_service.go:186-243`（`requestDB.Transaction` 内 `FindInBatches(100)` 遍历所有启用用户，逐批 `Create` 通知，全部在同一事务并持有 `task_claims` 行锁）
- Problem: 用户数增长后，单事务写入行数 = 用户数，事务时间与用户数线性相关，且发生在服务尚未监听的时候；若因超时（GORM `DefaultContextTimeout` 15s）或锁等待失败，`done` 未置位，下次重启会再次尝试，每次启动都被拖慢。
- Why it matters: 发布是最需要快速恢复的时刻；该逻辑让"用户越多，发版越慢、越容易失败"。
- Realistic failure scenario: 用户量到 10 万时发版：容器启动后 30s+ 未监听，Nginx 返回 502；若事务失败回滚，日志仅有"发布版本更新站内信失败"，而下一次重启再来一遍。
- Minimal fix: 放到后台 goroutine 中执行（服务先监听）；按批次提交（每批独立事务），用 `task_claims` 记录进度游标，保证幂等与可续跑。
- Better long-term fix: 改为"广播通知一行 + 用户已读表"的拉模式，不再按用户扇出写入。
- Regression test suggestion: 构造 5000 个用户，断言 `main` 的监听不等待通知写入完成；中途注入一次失败后重跑，断言每个用户恰好一条通知。
- Estimated effort: 半天

### Finding: F09 通知、行为事件、成就事件、聊天记录等表没有保留策略，无界增长

- Severity: Medium
- Confidence: High
- Category: Performance
- Status: Confirmed
- Affected area: `cmd/server/main.go` housekeeping
- Evidence:
  - File: `backend/cmd/server/main.go:102-124`（清理项：验证码、上传、`request_windows`、`assistant_leases`、`assistant_usages`、过期建议、180 天审计日志）
  - 未清理：`notifications`（每次发版 × 全体用户 + 每日买菜提醒）、`behavior_events`、`achievement_events`、`chat_messages`/`chat_sessions`。
- Problem: 这些表随时间与用户数线性增长，没有任何保留期；其中 `notifications` 由系统批量写入，增长最快。
- Why it matters: 一年后统计/列表查询（未读数、`ListNotifications` 的 `Count`）变慢，备份与迁移时间变长；SQLite 部署体积持续膨胀。
- Realistic failure scenario: 每周发版一次、1 万用户：一年约 52 万条系统通知，另有每日提醒；`GET /api/notifications` 的 `Count(unread)` 逐渐成为慢查询。
- Minimal fix: housekeeping 增加：已读通知 90 天、未读 180 天删除；`behavior_events` / `achievement_events` 保留 365 天；批量分段删除（每批 1000 行）以免长时间锁表。
- Better long-term fix: 系统广播改为拉模式（见 F08），行为事件按月汇总后删除明细。
- Regression test suggestion: 插入过期与未过期各 N 条，运行 housekeeping，断言只删除过期部分且分批执行。
- Estimated effort: 2–3 小时

### Finding: F10 `/api/me/export` 一次性把全部个人数据读进内存再序列化

- Severity: Medium
- Confidence: Medium
- Category: Performance
- Status: Suspected
- Affected area: `handlers/auth.go` 数据导出
- Evidence:
  - File: `backend/internal/handlers/auth.go:338-379`
  - Function / Module: `ExportMe`
  - Relevant behavior: `records`、`ratings`、`suggestions`、`sessions`、`messages`、`journal`、`notifications` 全部 `Find` 无上限（仅 `events` 限 10000），然后整体 `utils.Success` 序列化。
  - 并发由 `largeReadSlots(2)` 限制（`request_budget.go:47-54`）。
- Problem: 单条助手回复上限 16 KiB（`llm/client.go:246`），重度用户一年可累积上万条消息；对象加载 + JSON 编码会在内存中形成数倍副本。容器 `GOMEMLIMIT=192MiB` 且与 ASR 共享 1 GiB。内存量级为推算，故为 Suspected。
- Why it matters: 导出是用户权利接口，也最可能在账号注销前被调用；OOM 会杀掉整个 API 进程（包括正在进行的 SSE）。
- Realistic failure scenario: 两个重度用户同时导出（各含 1.5 万条消息 + 几千条记录）→ Go 堆瞬时超过 300 MiB，GC 抖动；若同时有 ASR 在跑，容器触发 OOM 重启。
- Minimal fix: 使用 `json.Encoder` 流式写出，逐表 `FindInBatches` 编码；或设置每表上限并在响应中标明截断。
- Better long-term fix: 导出改为异步任务，生成文件后通过通知提供下载链接。
- Regression test suggestion: 为用户生成 2 万条消息，导出时采样 `runtime.MemStats.HeapAlloc`，断言峰值低于阈值且输出为合法 JSON。
- Estimated effort: 半天

### Finding: F11 批量删除 500 道菜逐条独立事务并有 N+1 查询，单条失败被静默跳过

- Severity: Medium
- Confidence: High
- Category: Performance
- Status: Confirmed
- Affected area: `handlers/dish.go`、`services/dish_access.go`
- Evidence:
  - File: `backend/internal/handlers/dish.go:436-461`（`BatchDeleteDishes`）
  - File: `backend/internal/services/dish_access.go:31-56,70-120`（每道菜：`DishAccessFor` → `FamilyForUser`（2 次查询）+ `pendingDishRequestID`；`DeleteDishCascade` 独立事务 + 家庭锁 + 3 次删除 + `InvalidateWeekPlan` × 家庭成员数）
- Problem: 500 道菜 ≈ 500 个事务、数千次查询，在 15s 请求期限内可能被取消到一半；`err == nil` 才计数，其余错误完全吞掉，响应只返回 `deleted` 数，客户端无法知道哪些失败。
- Why it matters: 批量操作是用户最容易触发"极端输入"的地方；部分成功且无明细会造成列表与预期不一致。
- Realistic failure scenario: 管理员在后台全选 500 道公共菜删除 → 第 310 道时请求 15s 超时 → 前 309 道已删除、其余保留，前端仅提示"批量操作成功"并刷新列表，管理员不知道需要重试。
- Minimal fix: 循环外只查一次家庭与成员；按 `DeleteMode` 分组后在一个事务内批量删除；返回 `failed_ids`。
- Better long-term fix: 统一"批量操作结果"结构（成功/失败/跳过 ID 列表），前端据此提示。
- Regression test suggestion: 500 道菜批量删除，断言查询次数 < 50（GORM 回调计数）且在 2s 内完成；注入第 N 道失败，断言响应含 `failed_ids`。
- Estimated effort: 半天

### Finding: F12 批量改分类不做任何校验，写库失败也返回成功

- Severity: Medium
- Confidence: High
- Category: Maintainability
- Status: Confirmed
- Affected area: `handlers/dish.go` 批量接口（UI 筛选栏）
- Evidence:
  - File: `backend/internal/handlers/dish.go:468-478`（`BatchUpdateCategory`：`Category` 只要求非空，直接 `Update("category", req.Category)`，忽略 `.Error`，恒返回"批量修改分类成功"）
  - 对照 `backend/internal/handlers/dish_validation.go:29`：单条编辑限制分类 ≤40 字符。
  - `backend/internal/models/dish.go:16`：`Category string gorm:"index"`（MySQL 下 GORM 为带索引的字符串分配 `varchar(191)`）。
- Problem: 同一字段在单条与批量入口的校验不一致（DRY 违反，principle 2.x）。SQLite 下可写入任意长度分类；MySQL 下超过 191 字符会因严格模式报错，但错误被忽略并提示成功。
- Why it matters: 分类是菜谱列表、分类计数（`/dishes/category-counts`）和小程序分类标签的来源，脏值会直接破坏 UI 布局；"失败但提示成功"会误导用户。
- Realistic failure scenario: 用户批量把 30 道菜改为一个 80 字的分类名 → Web 端分类横向标签被撑破、小程序分类 chip 溢出；在生产 MySQL 上改成 300 字则全部失败但提示成功，用户以为已修改。
- Minimal fix: 复用 `dish_validation.go` 的长度规则（≤40，去空白），检查 `.Error` 与 `RowsAffected`。
- Better long-term fix: 分类改为引用站点 `categories` 设置中的受控值，或独立 `categories` 表。
- Regression test suggestion: `POST /api/dishes/batch-category` 传 41 字符分类断言 400；模拟 DB 错误断言 500。
- Estimated effort: 30 分钟

### Finding: F13 助手会话超过 200 条消息后，界面只显示最早的 200 条

- Severity: Medium
- Confidence: High
- Category: Maintainability
- Status: Confirmed
- Affected area: 助手会话（Web `AssistantChat.tsx`、小程序 `pages/chat`）
- Evidence:
  - File: `backend/internal/handlers/assistant.go:160`（`Order("id ASC").Limit(200)`）
  - 对照 `backend/internal/assistant/assistant.go:522`（模型上下文取 `Order("id DESC").Limit(historyMessages)`，即最新消息）
  - 小程序 `miniprogram/pages/chat/chat.js:171-186` 直接渲染返回的列表并滚动到底部。
- Problem: 长会话重新打开时，最新的问答不在返回结果中；用户看到的"最后一条"是很久以前的消息，而模型仍记得最新内容，形成界面与模型不一致。
- Why it matters: 用户会认为最新回复丢失，这是典型的数据截断型 UI 错乱。
- Realistic failure scenario: 用户每天在同一会话里问"今晚吃什么"，约 100 天（每次 2 条）后第 201 条及之后的消息从历史中消失；用户继续提问时，模型引用了界面上看不到的内容。
- Minimal fix: 改为 `Order("id DESC").Limit(200)` 后在内存中反转；返回 `has_more` 与游标。
- Better long-term fix: 历史消息分页（向上滚动加载更早消息）。
- Regression test suggestion: 为会话插入 250 条消息，断言接口返回的最后一条 `id` 为最大值、共 200 条、按时间升序。
- Estimated effort: 30 分钟（后端）+ 1–2 小时（分页 UI）

### Finding: F14 各类选择器只拉取第一页 100 道菜并在客户端过滤，超过 100 道后"搜不到"；`pageSize > 100` 还会静默回退为 20

- Severity: Medium
- Confidence: High
- Category: Maintainability
- Status: Confirmed
- Affected area: 明日菜单、家庭菜单/私房菜分享、菜谱详情"同类推荐"
- Evidence:
  - `frontend/src/pages/Tomorrow.tsx:497-500,511-518`：`pageSize: "100"`，`search`/`category` 只在内存中过滤。
  - `miniprogram/pages/tomorrow/tomorrow.js:294-302`：同样拉 100 条并按 meal 缓存，搜索在本地。
  - `frontend/src/pages/Family.tsx:43-45`（家庭菜谱、私房菜各 100）；`miniprogram/pages/family/family.js:385`（导入食材的菜谱列表 100 条，失败静默）。
  - `frontend/src/pages/DishDetail.tsx:147-152`（"同类推荐"基于前 100 道菜）。
  - `backend/internal/handlers/common.go:12-22`：`pageSize > 100` 时回退为默认值（20），而不是截断为 100。
  - 规模：种子公共菜约 100 道（`internal/dishes/pack_*.go` 共 100 个 `r(`），个人与家庭各可达 500 道（`services/dish_quota.go:10`）。
- Problem: 只要"可见菜谱"超过 100 道（公共 100 道 + 任意私房菜即会超过），排序靠后的菜谱在选择器中永远不出现，客户端搜索也搜不到。记录页等处的 `forDates` 做了分页循环（`utils/records.js:3-12`），说明问题已被意识到但未推广到选择器。
- Why it matters: 用户明明有这道菜，却无法在明日菜单/家庭菜单中选中，是明确的功能性 UI 错乱；而且问题只在数据量增长后才出现，测试环境难以发现。
- Realistic failure scenario: 用户新增了 30 道私房菜（`sort_order` 默认为 0，排在公共菜之后）→ 在"明日菜单"搜索自己的"外婆红烧肉"无结果；家庭页"导入食材"列表里也找不到。
- Minimal fix: 选择器把 `search`/`category` 传给后端（`/api/dishes` 已支持 `search`）；`pageParams` 对超过 100 的值截断为 100 而不是回退为 20。
- Better long-term fix: 统一的 `DishPicker` 组件/模块（Web 与小程序各一），服务端搜索 + 无限滚动。
- Regression test suggestion: 种子 100 道 + 用户 30 道，断言选择器搜索用户菜名能命中；`pageSize=200` 断言返回 100 条。
- Estimated effort: 半天

### Finding: F15 小程序编辑菜谱：视频提炼进行中选封面/切到后台会触发 onHide，静默取消且已计次

- Severity: Medium
- Confidence: Medium
- Category: Maintainability
- Status: Suspected
- Affected area: `miniprogram/pages/dish-edit`
- Evidence:
  - File: `miniprogram/pages/dish-edit/dish-edit.js:108`（`onHide() { this.cancelVideoImport(); this.flushDraft() }`）
  - `dish-edit.js:320-324`（`cancelVideoImport` 中止请求并把 `videoImportBusy` 置为 false，不提示用户）
  - `dish-edit.js:398-411`（`pickCover`/`pickExtra` 未检查 `videoImportBusy`，调用 `wx.chooseImage` 会打开系统相册，页面进入 onHide）
  - `dish-edit.js:310`（界面文案："开始 AI 处理后计 1 次"）
- Problem: 用户在等待提炼时去选封面、接电话或切到抖音核对视频，页面 onHide → 请求被中止；返回页面后既没有结果也没有错误提示，而服务端已扣减当日次数。标记 Suspected：`chooseImage` 触发 onHide 为微信已知行为，但未在真机复现。
- Why it matters: 用户体验上表现为"提炼莫名其妙没了、次数少了"，且与 AI 次数（助手共享配额）直接相关。
- Realistic failure scenario: 粘贴抖音链接 → 点"AI 提炼"（进度"正在读取视频内容…"）→ 顺手点"选封面" → 相册打开 → 返回后进度消失，表单未填充，"今日剩余"减少 1。
- Minimal fix: onHide 不再取消提炼（只在 onUnload 取消）；或者提炼进行中禁用选图按钮；被取消时显示"提炼已中断"。
- Better long-term fix: 提炼改为服务端任务 + 轮询/恢复，页面生命周期不影响结果。
- Regression test suggestion: 小程序自动化（miniprogram-automator）：开始提炼后触发 `pageHide` 再 `pageShow`，断言提炼继续或显示中断提示。
- Estimated effort: 1 小时

### Finding: F16 Web 记录页多图上传无上传态：上传中可保存导致照片丢失，连点"+"可超过 9 张

- Severity: Medium
- Confidence: High
- Category: Maintainability
- Status: Confirmed
- Affected area: `frontend/src/pages/History.tsx` 当日评价弹窗
- Evidence:
  - File: `frontend/src/pages/History.tsx:265-307`（`handlePhotoUpload` 逐张 `await uploadApi.image`，无 `uploading` 状态；`remaining = 9 - mealPhotos.length` 取自闭包中的旧值）
  - `History.tsx:669-672`（"保存评价"仅 `disabled={!mealRatingMood}`，不看上传进度）
  - `History.tsx:255-263`（`submitMealRating` 使用提交时刻的 `mealPhotos`）
- Problem: 上传是顺序的（每张数秒，且后端上传为全站单槽，见 F07），期间 UI 无任何进度；用户点保存时尚未完成的照片不会被提交；第一批未传完时再次点"+"，`remaining` 仍按旧数组计算，可超过 9 张上限。
- Why it matters: 照片是记录页的核心内容；"保存成功但少了照片"是典型的交互错乱，且已上传未引用的图片占用配额直到 24 小时 GC。
- Realistic failure scenario: 选择 6 张照片 → 第 2 张上传中点"保存评价" → 弹窗关闭，只保存了 1 张；随后剩余 4 张陆续上传完成但已无处显示。
- Minimal fix: 增加 `uploadingCount` 状态，上传中禁用"保存"和"+"，并显示"正在上传 2/6"；`remaining` 在 setState 回调中基于最新值计算。
- Better long-term fix: 抽出 Web 端通用的 `useImageUploads` hook（队列、进度、429 自动退避重试），各页面复用（小程序 `dish-edit` 已有 `uploadingExtra` 可借鉴）。
- Regression test suggestion: jsdom 测试 mock 上传延迟，断言上传中"保存"按钮 disabled；连续两次选择 6 张，断言最终 ≤9 张。
- Estimated effort: 2 小时

### Finding: F17 多处写库忽略错误仍返回成功（静默失败）

- Severity: Medium
- Confidence: High
- Category: Stability
- Status: Confirmed
- Affected area: handlers 层
- Evidence:
  - `backend/internal/handlers/auth.go:188-191`（`UpdateMe`：`Updates(updates)` 不检查 `.Error`，随后返回内存中已修改的 `userView`）
  - `backend/internal/handlers/dish.go:426-429`（`BatchToggleDishes`）、`dish.go:474-477`（`BatchUpdateCategory`）
  - `backend/internal/handlers/manage.go:114,130,134`（`UnlockAchievement` / `ToggleAchievement` 的 `Create`/`Delete`）
- Problem: DB 超时、锁等待或约束错误时，客户端收到成功响应及"已保存"的数据；刷新后回退。与本仓库其他地方（如 `ChangePassword` 的 CAS、`DeleteMe` 事务）的严谨程度不一致（Fail-fast 违反，principle 4.x）。
- Why it matters: 在 F05 描述的慢 DB 场景下，这类"假成功"会成批出现，用户无法信任界面状态。
- Realistic failure scenario: MySQL 短暂不可用时用户改昵称 → 接口返回新昵称 → 前端 store 更新 → 次日打开仍是旧昵称。
- Minimal fix: 所有写操作检查 `.Error`，失败返回 5xx；批量接口返回 `RowsAffected`。
- Better long-term fix: 在 lint 中加入自定义规则（如 `errcheck` 针对 GORM 链式调用的 `.Error` 未读取）。
- Regression test suggestion: 用 GORM 回调注入写失败，断言 `PUT /api/me` 返回非 0 code 且响应中昵称未变。
- Estimated effort: 2 小时

### Finding: F18 仓库没有 CI 流水线，测试、vet、race、漏洞扫描全靠人工执行

- Severity: Medium
- Confidence: High
- Category: Release
- Status: Confirmed
- Affected area: 发布流程
- Evidence:
  - 仓库根目录无 `.github/`、`.gitlab-ci.yml` 等 CI 配置（`ls -a` 已核对）；发布脚本仅 `scripts/next-version.sh`、`build_linux.bat`、`build_windows.bat`。
  - `docs/audit-remediation-2026-09-28.md:39-59`：`go test -race`、`govulncheck`、`npm audit` 均为手工执行的记录。
- Problem: 当前质量完全依赖发布者每次记得跑完全部检查；大量未提交改动（git status 中 50+ 个修改文件）进一步增加了遗漏风险。
- Why it matters: 本仓库的很多安全性质（竞态、锁顺序、MySQL 语义）只有测试能守住，没有 CI 就没有持续保证。
- Realistic failure scenario: 某次热修只改了一个 handler，发布者跳过 `go test -race`，引入的数据竞争在生产高峰时才暴露。
- Minimal fix: 添加 CI：`go vet`、`go test -race ./...`、`govulncheck`、`npm ci && npm run lint && npm test && npm run build`。
- Better long-term fix: CI 中起 MySQL 8 服务容器运行 `TestAuditMySQL`；镜像构建与 Trivy 扫描；main 分支保护。
- Regression test suggestion: CI 本身即回归；故意提交一个 race 用例验证流水线会失败。
- Estimated effort: 半天

### Finding: F19 测试信心的盲区：MySQL 并发语义默认跳过，小程序零自动化，Web 仅有少量脚本测试

- Severity: Medium
- Confidence: High
- Category: Testing
- Status: Confirmed
- Affected area: 测试体系
- Evidence:
  - `backend/internal/testutil/testutil.go:22-28`：集成测试统一使用 SQLite。
  - `backend/internal/services/audit_mysql_test.go`：仅在提供 `ARRE_AUDIT_MYSQL_SOCKET` 时运行（`docs/audit-remediation-2026-09-28.md:54-59`）。
  - `frontend/package.json:10`：`"test": "node --test ../scripts/tests/*.test.cjs"`，没有组件/交互测试框架。
  - `miniprogram/`：无任何测试文件。
- Problem: 生产是 MySQL（RR 隔离、`FOR UPDATE`、`GET_LOCK`），而代码中大量正确性依赖这些语义（`family_service.go:233-236` 的注释即为证据）；默认测试在 SQLite 的串行写锁下必然"通过"。UI 交互类问题（F13–F16、F20）没有任何自动化防线。
- Why it matters: 这正是本次发现的 UI 截断与交互问题能进入生产的原因。
- Realistic failure scenario: 未来修改 `LockFamilyForUser` 的锁顺序，在 SQLite 上测试全绿，但在 MySQL 上转让/退出并发时出现死锁。
- Minimal fix: CI 中常驻运行 MySQL 用例（见 F18）；Web 引入 Vitest + Testing Library 覆盖上传、选择器、聊天流。
- Better long-term fix: 小程序使用 `miniprogram-automator` 为聊天、编辑菜谱、家庭页编写冒烟测试。
- Regression test suggestion: 见上。
- Estimated effort: 1–2 天

### Finding: F20 小程序家庭页刷新无请求序号，勾选清单无乐观更新，并发刷新可能旧数据覆盖

- Severity: Low
- Confidence: Medium
- Category: Maintainability
- Status: Confirmed
- Affected area: `miniprogram/pages/family/family.js`
- Evidence:
  - `family.js:44-85`：`onShow` 每次都 `load()`，下拉刷新、各操作完成后也 `load()`，没有请求序号（对比 `chat.js` 使用 `_request` 序号）。
  - `family.js:360-369`：`toggleItem` 每次请求后整表 `loadShopping()`，没有乐观更新；`checked` 来自 dataset，在刷新返回前连点两次会发送相同的目标值。
  - `family.js:381-390`：`openImport` 失败静默，`_dishesLoaded` 在页面生命周期内永久缓存。
- Problem: 快速操作时，较早发出的 `load()` 可能晚于较新的返回并覆盖状态（例如刚同意的删除申请又出现）；勾选有明显延迟与"点了没反应"的体验。
- Why it matters: 家庭页是多人协作页面，状态闪回会让用户误以为操作失败并重复操作。
- Realistic failure scenario: 同意删除申请后立即下拉刷新 → 两次 `load` 并发 → 旧响应后到，申请重新出现在列表中，几秒后再次消失。
- Minimal fix: `load`/`loadShopping` 引入递增序号，只应用最新响应；勾选先本地切换、失败回滚。
- Better long-term fix: 把"序号化请求"封装进 `utils/api.js`（如 `latest(key, promise)`）供所有页面使用。
- Regression test suggestion: 自动化测试中让第一次 `/family` 响应延迟 1s，断言最终渲染的是第二次响应。
- Estimated effort: 1–2 小时

### Finding: F21 家庭菜谱删除申请"先查后建"且无唯一约束，并发可产生重复待处理申请

- Severity: Low
- Confidence: High
- Category: Stability
- Status: Confirmed
- Affected area: `services/dish_access.go`
- Evidence:
  - File: `backend/internal/services/dish_access.go:191-197`（先 `First` 查 pending，再 `Create`；`DishDeleteRequest` 没有 `(dish_id, status)` 唯一约束，见 `models/` 的 `uniqueIndex` 列表）
- Problem: 双击或网络重试时生成两条 pending 申请，管理员会收到两条通知；批准一条后另一条由 `DeleteDishCascade` 自动置为 approved，结果正确但通知重复。
- Why it matters: 轻微的数据与通知重复。
- Realistic failure scenario: 成员在弱网下双击"申请删除"→ 管理员收到两条"菜谱删除申请"站内信。
- Minimal fix: 在家庭行锁（`lockFamily`）事务内执行查重与创建。
- Better long-term fix: 为 pending 状态建立部分唯一约束（MySQL 可用生成列实现）。
- Regression test suggestion: 并发 10 次 `RequestDishDeletion`，断言 pending 恰好 1 条、通知 1 条。
- Estimated effort: 30 分钟

### Finding: F22 助手会话列表只返回最近 50 个且无分页

- Severity: Low
- Confidence: High
- Category: Maintainability
- Status: Confirmed
- Affected area: `handlers/assistant.go`
- Evidence:
  - File: `backend/internal/handlers/assistant.go:137`（`Order("updated_at DESC").Limit(50)`）；小程序 `chat.js:158-169` 与 Web 历史面板直接展示。
- Problem: 第 51 个及更早的会话在 UI 中不可见也无法删除，但仍占用存储并出现在导出中。
- Why it matters: 轻度数据不可达。
- Realistic failure scenario: 用户建立过 80 个会话，想找两个月前的一次对话，历史列表中不存在。
- Minimal fix: 支持 `page`/`before` 参数，前端"加载更多"。
- Better long-term fix: 会话自动归档与清理策略（配合 F09）。
- Regression test suggestion: 建 60 个会话，断言第二页返回 10 个。
- Estimated effort: 1 小时

### Finding: F23 小程序导航栏尺寸只在启动时计算一次，窗口尺寸变化后可能错位

- Severity: Low
- Confidence: Low
- Category: Maintainability
- Status: Suspected
- Affected area: `miniprogram/app.js`、`components/nav-bar`
- Evidence:
  - File: `miniprogram/app.js:13-31,55-65`（`readNavMetrics` 基于胶囊按钮与窗口宽度，结果缓存在 `globalData.nav`，没有监听 `wx.onWindowResize`）
- Problem: iPad 横竖屏切换、PC/Mac 微信调整窗口、安卓分屏时，胶囊位置与窗口宽度改变，自定义导航栏的 `capsuleSpace`/高度仍为旧值。未真机验证，标记 Suspected。
- Why it matters: 标题与胶囊重叠或错位属于可见的布局错乱。
- Realistic failure scenario: 在 Mac 微信中打开小程序并拉宽窗口，右上角标题区域与胶囊按钮重叠。
- Minimal fix: `wx.onWindowResize` 时清空 `globalData.nav` 并通知当前页面重算。
- Better long-term fix: nav-bar 组件内部自行读取并订阅尺寸变化。
- Regression test suggestion: 在开发者工具中切换设备与横屏，截图对比导航栏。
- Estimated effort: 1 小时

### Finding: F24 助手/视频提炼在模型调用失败时不返还已扣次数

- Severity: Low
- Confidence: High
- Category: Stability
- Status: Confirmed
- Affected area: `handlers/assistant.go`、`handlers/video_recipe.go`
- Evidence:
  - File: `backend/internal/handlers/assistant.go:97-132`（`ConsumeAssistantQuota` 在 `assistant.Run` 之前执行；`Run` 返回错误时仅发送 `error`/`done` 事件）
  - `backend/internal/handlers/video_recipe.go:135`（同样在处理前扣次数）
- Problem: LLM 超时、上游 5xx、客户端中止（见 F15）都会消耗用户每日 20 次额度与站点 200 次额度。
- Why it matters: 上游不稳定时用户额度被快速"吃掉"，站点额度也可能被故障耗尽。
- Realistic failure scenario: DeepSeek 故障 10 分钟，用户重试 5 次均失败，当日剩余 15 次。
- Minimal fix: 上游错误（非内容审核拒绝）时在同一日期行上 `used = used - 1`（带 `used > 0` 条件）。
- Better long-term fix: "预留 → 确认/释放"两阶段配额。
- Regression test suggestion: mock LLM 返回 503，断言配额未减少。
- Estimated effort: 1 小时

### Finding: F25 管理员可配置的 LLM Base URL 未经公网地址校验，可用于探测内网

- Severity: Low
- Confidence: High
- Category: Security
- Status: Confirmed
- Affected area: `llm/client.go`、`/api/admin/llm/test`
- Evidence:
  - File: `backend/internal/llm/client.go:239-243,258`（普通 `http.Client`，只禁止重定向，没有 `GuardPublicAddr`）
  - `backend/internal/llm/client.go:47-57`（`Resolve` 读取后台设置 `llm_base_url`）
- Problem: 管理员（或管理员账号被盗后）可将 base URL 指向 `http://mysql:3306`、`http://garage:3900`、`http://host.docker.internal:*` 并通过"测试连接"的返回状态码探测内网。需要管理员权限，故为 Low。
- Why it matters: 扩大管理员账号被盗后的横向移动面（配合 F02 更明显）。
- Realistic failure scenario: 获得管理员 JWT 后，逐个测试内网主机端口的 `AI 服务暂时不可用（状态 N）` 与"连接失败"差异。
- Minimal fix: 保存设置时校验 scheme 为 https（CPA 本地地址除外，可配置白名单）；测试接口返回统一错误。
- Better long-term fix: LLM base URL 只从环境变量/挂载文件读取，不允许在后台修改。
- Regression test suggestion: `PUT /api/admin/settings` 设置 `http://127.0.0.1:3306` 断言 400。
- Estimated effort: 1 小时

### Finding: F26 超大文件与多入口重复的业务逻辑增加修改成本

- Severity: Low
- Confidence: High
- Category: Maintainability
- Status: Confirmed
- Affected area: 前后端多个模块
- Evidence:
  - `frontend/src/pages/Tomorrow.tsx`：1028 行；`History.tsx` 685 行；`DishList.tsx` 662 行
  - `backend/internal/services/plan_service.go` 825 行、`achievement_service.go` 819 行、`handlers/agent.go` 767 行、`services/family_service.go` 655 行
  - Web 与小程序各自实现相同业务（明日菜单、家庭页、上传、选择器），如 F14 的选择器问题需在 4 处分别修复。
- Problem: 文件规模超过 principle 1.x 建议；同一需求多处实现，易出现 F14 这类"修了一处漏了三处"的问题。
- Why it matters: 维护成本与行为漂移风险。
- Realistic failure scenario: 修复 Web 选择器的服务端搜索后，小程序仍是客户端搜索，用户在两端看到不同结果。
- Minimal fix: 拆分 `Tomorrow.tsx`（选择器、计划卡片、确认弹窗为独立组件）；将选择器/上传逻辑抽到共享模块。
- Better long-term fix: 为 Web 与小程序定义同一份"业务契约"文档与接口，关键逻辑放在服务端。
- Regression test suggestion: 拆分后的组件各自有单元测试。
- Estimated effort: 2–3 天（渐进）

### Finding: F27 依赖中有停止维护的图像库，纯 Go SQLite 驱动随生产二进制发布

- Severity: Low
- Confidence: Medium
- Category: Release
- Status: Confirmed
- Affected area: `backend/go.mod`
- Evidence:
  - `backend/go.mod:6`：`github.com/disintegration/imaging v1.6.2`（上游最后一次发布为 2020 年）。
  - `backend/go.mod:8,24,55`：`glebarez/sqlite v1.11.0` → `modernc.org/sqlite v1.23.1`（2023 年），生产使用 MySQL，但驱动与 libc 转译代码仍编入 `ninimenu` 二进制。
  - `backend/go.mod:28`：间接依赖 `go-sql-driver/mysql v1.7.0`（较旧）。
- Problem: 图像解码是处理不可信输入的路径，依赖已停更的库意味着新漏洞无人修复；SQLite 驱动增加二进制体积与攻击面，但生产用不到。
- Why it matters: 长期供应链风险；此前 x/image 已出现过 WebP 相关公告（见 remediation F01）。
- Realistic failure scenario: 未来 `imaging` 的某个解码/缩放路径被发现内存问题，上游无修复版本可升级。
- Minimal fix: 升级 `go-sql-driver/mysql` 与 `modernc.org/sqlite`；评估用 `golang.org/x/image/draw` 替换 `imaging` 的缩放与 EXIF 方向处理。
- Better long-term fix: 通过 build tag 让生产构建不包含 SQLite 驱动。
- Regression test suggestion: `imaging/compress_test.go` 覆盖替换后的方向修正与缩放。
- Estimated effort: 半天

### Finding: F28 CORS 默认值为 `*`

- Severity: Info
- Confidence: High
- Category: Security
- Status: Confirmed
- Affected area: `config.go`
- Evidence:
  - `backend/internal/config/config.go:108`（`CORS_ORIGINS` 默认 `*`）；`docker-compose.yml:51` 与 `.env.example:9` 覆盖为站点域名。
- Problem: 认证使用 Bearer 头而非 Cookie，`*` 不直接导致 CSRF；但若直接运行二进制（`build_linux.bat` 产物）未设置该变量，任意网页都可跨域读取公开接口。
- Why it matters: 防御纵深，属于"安全默认值"问题。
- Realistic failure scenario: 非 Compose 部署时漏配，第三方页面可跨域调用公开 API。
- Minimal fix: 生产环境默认使用 `PUBLIC_URL`，未配置则拒绝跨域。
- Better long-term fix: 纳入 F02 的 `config.Validate()`。
- Regression test suggestion: 生产模式未设置 `CORS_ORIGINS` 时断言 `Access-Control-Allow-Origin` 不为 `*`。
- Estimated effort: 30 分钟

### Finding: F29 多副本部署下，所有进程内上限与注释描述的"单实例"前提不一致

- Severity: Info
- Confidence: High
- Category: Maintainability
- Status: Confirmed
- Affected area: 架构
- Evidence:
  - `backend/internal/middleware/middleware.go:200`（注释"进程内即可满足单实例部署"）
  - `.env.example:24`（"Set false on API replicas after the one-off DB_MIGRATE_ONLY migration."）
  - 进程内：`apiSlots(16)`、`originSlots(8)`、`imageUploadSlot(1)`、`resourcebudget.slot(1)`、`windowLimiter` 多个实例、成就 worker 队列。
- Problem: 一部分限制（助手租约、配额、`ReserveWindow`）已做成数据库共享，另一部分仍是每进程；扩到 N 个副本时，前者总量不变，后者总量变为 N 倍，其中 `resourcebudget` 的"内存保护"在同一主机上会失效。
- Why it matters: 水平扩展时的容量模型不可预测，是后续扩容前需要明确的架构决策。
- Realistic failure scenario: 为扛流量加到 3 个副本放在同一台 4 GiB 主机上，3 个进程同时做图片解码 + ASR，超过主机内存。
- Minimal fix: 在部署文档中明确"单副本"或"每主机一个副本"的约束，并修正注释。
- Better long-term fix: 内存敏感的准入（重任务）改为基于 DB 或主机级信号量；其他进程级上限作为"每副本容量"显式配置。
- Regression test suggestion: 无（架构约定），可在部署检查脚本中校验副本数。
- Estimated effort: 1 小时（文档）

## 5. Security Concerns

**认证与会话**
- F02 占位密钥通过启动检查（High）
- F03 PAT 不随改密失效（Medium）
- F04 验证码尝试计数竞态（Medium）

**可用性攻击面（未鉴权）**
- F01 限流表满即全站拒绝（High）
- F05 16 并发槽（Medium）、F06 图片回源 8 槽（Medium）

**SSRF / 配置**
- F25 管理员 LLM Base URL（Low）、F28 CORS 默认值（Info）

已核查、未发现问题：
- [x] 密码：PBKDF2-SHA512 21 万次、16 字节盐、常量时间比较、长度上限 128（`auth/password.go`）
- [x] JWT：限定 HS256、校验 issuer/exp/iat，`TokenVersion` 撤销，用户禁用即时生效（`auth/token.go`、`middleware.go:33-85`）
- [x] 改密 CAS、禁用递增版本、注销事务完整（`handlers/auth.go:194-335`）
- [x] 所有 Agent/MCP 工具都声明 scope，`agent.Invoke` 统一校验并审计；REST 路由按 scope 挂载（`agent/registry.go`、`routes.go:193-224`）
- [x] 用户 JWT 不能用于 Agent 接口，PAT 不能用于 App 接口（`middleware.go:56-61,162-167`）
- [x] 家庭邀请：24 字节随机、哈希存储、绑定邮箱、7 天过期、行锁下消费（`family_service.go:198-348`）
- [x] 批量改/删按归属过滤（`editableIDs` 与 `VisibleDishes`），GORM 对含 OR 的条件自动加括号
- [x] 路径 ID 仅允许纯数字（`middleware/path_ids.go`）
- [x] 视频/链接预览 SSRF：dial 阶段校验公网 IP、禁重定向与代理、端口与域名白名单、响应体上限（`video/http.go`）
- [x] 上传：内容嗅探、DecodeConfig 限像素、随机文件名、`KeyFromURL`+`OwnedBy` 校验删除权限、`nosniff`（`handlers/upload.go`、`imaging/compress.go`、`storage/store.go`）
- [x] Web 端不使用 `dangerouslySetInnerHTML`；登录 `redirect` 经 `safeRedirect`（`Login.tsx:26`）
- [x] Web 端每标签页会话世代、切换账号清空 react-query 缓存（`api/client.ts`、`App.tsx:60`）
- [x] 小程序请求检测令牌变化、401 统一回登录；草稿按用户 ID 分键（`utils/api.js`、`utils/recipe-draft.js`）
- [x] 敏感文件：`.env`、`secrets/`、`docs/private.*.key` 均被 `.gitignore` 与 `.dockerignore` 排除；镜像以 UID 1000 运行、`cap_drop: ALL`

## 6. Stability Concerns

- F08 启动路径单事务全员通知（Medium）
- F17 写库吞错仍返回成功（Medium）
- F11 批量删除部分成功无明细（Medium）
- F21 重复删除申请（Low）
- F24 失败不返还额度（Low）

已核查、未发现问题：
- [x] 所有外部调用都有超时：LLM 75s、SMTP 20s、微信 8s、视频 25s（dial 4s/TLS 4s/首字节 8s）、S3 回源 20s
- [x] 请求上下文贯穿 DB（GORM `DefaultContextTimeout` 15s + 路由级超时）
- [x] 优雅关闭：Shutdown 100s + 取消请求上下文 + 等待重任务释放 + compose stop grace 110s
- [x] 工具调用 `safeCall` 使用 `recover`；handler 由 `gin.Recovery` 兜底
- [x] 共享限流 `ReserveWindow`、助手租约、日配额均为条件更新，DB 失败时 fail-closed
- [x] MySQL 迁移锁固定连接、短轮询不超驱动读期限（`database/migration_lock.go`）
- [x] 小程序聊天流：请求序号、60ms 合并 setData、onUnload abort、UTF-8 分块解码处理截断字节（`pages/chat/chat.js`）

## 7. Performance Concerns

- F05 16 并发槽、F06 回源 8 槽、F07 上传 1 槽（Medium）
- F09 表无界增长（Medium）
- F10 导出全量加载（Medium）
- F11 批量删除 N+1（Medium）

已核查、未发现问题：
- [x] JSON 请求体 64/256 KiB 上限与结构深度/节点数上限（`request_budget.go:69-92,136-164`）
- [x] 图片解码前检查像素与边长上限，透明图复用像素缓冲
- [x] LLM 响应总量 1 MiB、正文 16 KiB、工具调用 8 个/32 KiB
- [x] 大读取接口（`/export`、`/api/dishes`）有独立并发上限 2
- [x] 管理员用户列表的记录数统计为一次 GROUP BY（非 N+1）
- [x] 视频缓存有界（128 项 / 20 MiB）

## 8. Testing Gaps

- F19 MySQL 语义、UI 交互、小程序无自动化（Medium）
- F18 无 CI（Medium）

已核查：
- [x] `go vet ./...` 通过；`go test ./...` 本机全部通过（SQLite）
- [x] 路由级回归测试覆盖 AI 安全、配额、家庭健康、菜谱可见性、上传、视频（`internal/routes/*_test.go`）
- [x] 存在竞态与锁顺序的专门测试（`services/audit_hardening_test.go`、`audit_mysql_test.go`）

## 9. Maintainability Concerns

- F26 超大文件与双端重复实现（Low）
- F12、F14 同一规则多处实现不一致（Medium）
- F29 进程内状态与多副本注释矛盾（Info）

## 10. Type Safety Concerns

未发现导致真实风险的类型问题，记录如下观察：
- `handlers/mcp.go:353`：`a["name"].(string)` 为不带 ok 的类型断言，但数据来自编译期常量 `prompts`，不可被外部输入影响。
- `middleware.go:110`：PAT scopes 反序列化错误被忽略，结果为空权限集（fail-closed），可接受。
- 状态字段为字符串常量（`"pending"`、`"approved"`、`"direct"` 等），集中定义在 `dish_access.go:17-26`，风险可控。

## 11. Release Concerns

- F18 无 CI（Medium）
- F27 依赖停更/冗余驱动（Low）
- F02 配置非 fail-fast（High，见安全）

已核查、未发现问题：
- [x] 多阶段构建、固定 Go 1.25.14、pip `--require-hashes`、ASR 模型 lock 文件校验
- [x] 非 root、`no-new-privileges`、`pids_limit`、内存/CPU 限制、tmpfs `noexec`、日志轮转 10m×3
- [x] 独立迁移流程与 `DB_AUTO_MIGRATE=false` 的副本配置有文档（`docs/deployment.md`）
- [x] `APP_VERSION` 构建参数注入，发布通知按版本幂等

---

## 12. Principles Compliance

整体上，本仓库在"原子性、锁顺序、上下文传递、最小权限"等方面明显高于平均水平，问题集中在**配置的 fail-fast**、**同一规则多处实现**以及**"限流上限"的失败语义**。

### Principles Violated

| Principle | Violations | Severity | Affected Areas |
|-----------|------------|----------|----------------|
| Fail-Fast | 3 | High | `config.Load`（F02、F28）、`pageParams` 静默回退（F14） |
| Graceful Degradation（上限应降级而非全拒） | 4 | High | `windowLimiter`（F01）、`apiSlots`（F05）、`originSlots`（F06）、`resourcebudget`（F07） |
| DRY / Single Source of Truth | 3 | Medium | 分类校验（F12）、选择器 Web/小程序多处实现（F14、F26）、上传逻辑多处实现（F16） |
| Command-Query / 错误传播 | 4 | Medium | `UpdateMe`、批量接口、成就接口吞错（F17） |
| Single Responsibility / 文件规模 | 5 | Low | `Tomorrow.tsx`、`plan_service.go`、`achievement_service.go`、`handlers/agent.go`、`family_service.go`（F26） |
| Bounded Growth / 数据生命周期 | 2 | Medium | housekeeping 缺表（F09）、启动扇出写入（F08） |
| Documentation Accuracy | 1 | Info | "单实例"注释 vs 多副本（F29） |

### Principles Respected

- **最小权限与纵深防御**：用户令牌、Agent 会话、PAT 三类凭证边界清晰，scope 在 REST 与 MCP 两个入口使用同一个注册表。
- **原子性**：配额、限流、租约、改密都使用条件更新而不是先读后写（F04 是少数例外）。
- **一致的锁顺序**：家庭相关操作统一"family → member/user"，并在 MySQL RR 下做当前读复查。
- **上下文与超时贯穿**：从 HTTP 到 DB、LLM、SMTP、S3 全链路传递 context。
- **安全默认的网络客户端**：视频客户端禁用代理、Cookie jar 与重定向，按 IP 在 dial 时校验。

---

## 12a. Fallback / Defensive Code Analysis

### Fallback Summary

| Subtype | Count | KeepWithAlert | FailFast | Remove |
|---------|-------|---------------|----------|--------|
| SilentFallback | 3 | 1 | 2 | 0 |
| EmptyCatch | 3 | 1 | 2 | 0 |
| CompatibilityBranch | 2 | 2 | 0 | 0 |
| SilentCorrection | 2 | 0 | 2 | 0 |
| DefensiveGuess | 1 | 1 | 0 | 0 |

- **SilentFallback → FailFast**：`config.Load` 对默认/占位密钥只警告（F02）；`pageParams` 对 `pageSize>100` 回退为 20（F14）。**KeepWithAlert**：`database.GetSetting` 出错返回 fallback（`database.go:376-387`），合理但应在非 NotFound 错误时记录日志。
- **EmptyCatch → FailFast**：`UpdateMe`、`BatchToggleDishes`/`BatchUpdateCategory`、`UnlockAchievement`/`ToggleAchievement` 忽略 `.Error`（F17）。**KeepWithAlert**：`BatchDeleteDishes` 跳过失败项应返回 `failed_ids`（F11）。
- **CompatibilityBranch（保留并标注移除时间）**：`loadDotEnv` 读取 `env.bak`（`config.go:230`）；多组旧环境变量别名（`SMTP_USERNAME`、`COOK_S3_*`、`DEEPSEEK_API_KEY`、`CPA_*`）。
- **SilentCorrection → FailFast**：`JWT_EXPIRE < 1h` 被静默改为 30 天（`config.go:175-177`）；`getEnvInt` 解析失败静默使用默认值（`config.go:265-273`）。
- **DefensiveGuess（保留）**：`LoginWithWechat` 任意 DB 错误都当作"未绑定"返回 `need_bind`（`auth_service.go:328-330`），建议区分 NotFound 与其他错误。

## 12b. Testing Authenticity Analysis

### Confidence Assessment

| Test Area | Real Confidence | Risk | Action |
|-----------|---------------|------|--------|
| `internal/routes/*_test.go`（真实路由 + SQLite） | High | MySQL 专有语义不覆盖 | Keep |
| `services/audit_hardening_test.go`（确定性交错） | High | — | Keep |
| `services/audit_mysql_test.go` | High（运行时） / None（默认跳过） | 默认 CI 下不执行 | Keep，纳入 CI |
| `middleware/ai_guard_test.go` | Medium | 未覆盖限流表容量满（F01） | 补充 |
| `handlers/upload_test.go`、`routes/upload_test.go` | Medium | 未覆盖与 ASR 槽冲突（F07） | 补充 |
| `assistant/*_test.go`（护栏、视频证据） | High | — | Keep |
| `frontend` `node --test ../scripts/tests` | Low | 不覆盖组件与交互 | 引入组件测试 |
| 小程序 | None | F13–F16、F20 类问题无防线 | 新增冒烟测试 |

### Valuable Tests

- 路由级集成测试走真实中间件链与数据库，能捕获鉴权/授权回归。
- 并发与幂等测试（成就、配额、通知、家庭转让）直接断言不变量。

### Suspicious Tests

- 未发现过度 mock；主要问题是"测试数据量太小"：没有测试构造 >100 道菜或 >200 条消息的数据集，因此 F13、F14 无法被捕获。

### Missing Tests

- 限流器容量上限行为（F01）、生产配置校验（F02）、PAT 随改密撤销（F03）、验证码并发猜测（F04）、会话历史截断（F13）、选择器大数据集（F14）、Web 上传交互（F16）。

---

## 12c. Type Safety Analysis

### Summary

| Subtype | Count | Critical | High | Medium | Low |
|---------|-------|----------|------|--------|-----|
| UnsafeBlock | 0 | 0 | 0 | 0 | 0 |
| TypeAssertion | 1 | 0 | 0 | 0 | 1 |
| InputBoundary | 1 | 0 | 0 | 1 | 0 |
| OutputLeak | 0 | 0 | 0 | 0 | 0 |
| BooleanTrap | 1 | 0 | 0 | 0 | 1 |
| StringlyTyped | 1 | 0 | 0 | 0 | 1 |
| ErrorType | 0 | 0 | 0 | 0 | 0 |

- **TypeAssertion**：`handlers/mcp.go:353` 不带 ok 的断言（常量数据，Low）。
- **InputBoundary**：`BatchUpdateCategory` 的 `Category` 未在边界校验（F12，Medium）。
- **BooleanTrap**：`LockFamilyForUser(tx, uid, false/true)` 的 `ownerOnly` 布尔参数（`family_service.go:353`），调用处可读性差（Low）。
- **StringlyTyped**：删除申请/建议状态为字符串（Low，已集中定义常量）。
- 错误类型使用哨兵错误 + `errors.Is/As`，未发现字符串比较错误。

## 12d. Frontend State Analysis

### Summary

| Subtype | Count | Affected Components |
|---------|-------|-------------------|
| ComponentSize | 4 | `Tomorrow.tsx`(1028)、`History.tsx`(685)、`DishList.tsx`(662)、`DishDetail.tsx`(527) |
| StateDuplication | 1 | Web/小程序各自缓存选择器数据（`_pickerCache`、`_dishesLoaded`） |
| PropDrilling | 0 | — |
| EffectChain | 0 | — |
| UIBusinessCoupling | 2 | `History.tsx` 上传逻辑内联在页面；`dish-edit.js` 生命周期直接控制 AI 任务（F15） |
| DOMasState | 0 | — |
| RequestState | 4 | 缺上传态（F16）、缺请求序号（F20）、截断列表（F13、F14） |
| RenderPerf | 0 | 小程序聊天已合并 setData；Web 使用 react-query 缓存 |

**UI 组件交互专项结论（用户重点关注）**

| 问题 | 端 | 复现条件 | 编号 |
|---|---|---|---|
| 长会话最新消息不显示 | Web + 小程序 | 会话 >200 条 | F13 |
| 选择器搜不到菜 | Web + 小程序 | 可见菜谱 >100 道 | F14 |
| 分类标签被撑破 / 改分类假成功 | Web + 小程序 | 批量改为超长分类 | F12 |
| 视频提炼被选图静默取消 | 小程序 | 提炼中点"选封面"或切后台 | F15 |
| 上传中保存丢照片 / 超 9 张 | Web | 多图上传过程中操作 | F16 |
| 照片墙随机破图 | Web + 小程序 | 冷缓存首屏大量用户图片 | F06 |
| 上传提示"正在处理视频" | Web + 小程序 | 他人正在做 ASR | F07 |
| 家庭页状态闪回、勾选延迟 | 小程序 | 快速操作 + 下拉刷新 | F20 |
| 导航栏错位 | 小程序 | iPad/PC 调整窗口 | F23 |

已核查、未发现问题：
- [x] 小程序 25 个页面的自定义组件（`nav-bar`、`dish-image`、`dish-card`、`tag-input`、`sheet`/`empty`/`confetti`）均在各自页面 `.json` 注册，`request-state` 全局注册
- [x] 聊天页中途切换会话/新建会话会 abort 并递增请求序号，不会串会话
- [x] Web 聊天与视频提炼在卸载时 abort；Web 请求在账号切换后拒绝迟到响应
- [x] 小程序编辑菜谱：保存中禁用重复提交，上传中禁止保存，链接预览只回填仍一致的输入
- [x] 家庭页创建/加入/邀请/导入/审批均有 `busy` 锁；破坏性操作有确认框

## 12e. Backend API Analysis

### Summary

| Subtype | Count | Affected Endpoints |
|---------|-------|-------------------|
| ApiConsistency | 2 | `pageSize` 语义（F14）；限流拒绝统一为 42900"过于频繁"，与"服务繁忙"混用（F05） |
| Validation | 1 | `POST /api/dishes/batch-category`（F12） |
| Auth | 1 | PAT 生命周期（F03） |
| NplusOne | 2 | `POST /api/dishes/batch-delete`（F11）、`SendShoppingReminders` 每用户 4+ 次查询 |
| Caching | 1 | `/uploads/*` 回源无缓存层（F06） |
| ErrorResponse | 1 | 批量接口不返回失败明细（F11、F17） |
| BusinessLogic | 2 | 失败不返还额度（F24）、重复删除申请（F21） |
| DataFlow | 2 | 会话历史截断（F13）、导出全量加载（F10） |

## 12f. Dependency Weight Analysis

### Dependency Scoreboard

| Dependency | Status | Weight | Transitives | Used For | Recommended Action |
|------------|--------|--------|-------------|----------|-------------------|
| gin-gonic/gin v1.12.0 | Healthy | 中 | sonic、validator、quic-go 等 | HTTP 框架 | Keep |
| gorm.io/gorm v1.31.1 + driver/mysql v1.5.7 | Healthy | 中 | go-sql-driver/mysql v1.7.0（旧） | ORM | Keep，升级间接 mysql 驱动 |
| glebarez/sqlite v1.11.0 | Overweight（生产不用） | 大（modernc libc 转译） | modernc.org/sqlite v1.23.1 等 | 开发/测试 SQLite | 用 build tag 从生产构建剔除 |
| disintegration/imaging v1.6.2 | Dead（上游停更） | 小 | — | EXIF 方向 + Lanczos 缩放 | 评估替换 |
| golang.org/x/image v0.45.0 | Healthy | 小 | — | WebP 解码 | Keep |
| golang-jwt/jwt/v5 v5.3.1 | Healthy | 小 | — | JWT | Keep |
| goccy/go-yaml v1.19.2 | Healthy | 小 | — | 解析 CPA 配置 | Keep |
| 前端 react 19 / react-query 5 / zustand 5 / axios 1 / gsap 3 | Healthy | 中（gsap 动画较重） | — | UI | Keep；gsap 可按页懒加载 |

---

## 13. Recommended Fix Order

### Fix Immediately

| 编号 | 事项 | 估时 |
|---|---|---|
| F02 | 生产配置 fail-fast（拒绝默认/占位/过短密钥） | 1h |
| F01 | 限流器容量满时淘汰而非拒绝；IPv6 按 /64 归一 | 2–4h |

### Fix Before Stable Release

| 编号 | 事项 | 估时 |
|---|---|---|
| F13 | 会话历史取最新 200 条 | 0.5h |
| F14 | 选择器服务端搜索；`pageSize` 截断为 100 | 0.5d |
| F12 + F17 | 批量分类校验；写库错误不再吞掉 | 2.5h |
| F03 | 改密/退出所有设备同时撤销 PAT | 2h |
| F04 | 验证码尝试计数原子化 | 1h |
| F05 | 长连接独立并发池、槽满短暂排队、健康检查暴露拒绝率 | 0.5d |
| F07 | 重任务槽带超时等待，上传读体与 S3 写入不持槽 | 0.5d |
| F15 + F16 | 小程序提炼不因 onHide 取消；Web 上传态 | 3h |
| F18 | 建立 CI（含 race、govulncheck、前端构建） | 0.5d |

### Schedule Later

| 编号 | 事项 | 估时 |
|---|---|---|
| F06 | `/uploads` 反代缓存或 CDN 直出 | 2–4h + 运维 |
| F08 | 发布通知后台分批、可续跑 | 0.5d |
| F09 | 数据保留策略 | 2–3h |
| F10 | 导出流式/异步化 | 0.5d |
| F11 | 批量删除批处理与失败明细 | 0.5d |
| F19 | MySQL 用例进 CI、Web 组件测试、小程序冒烟 | 1–2d |
| F20、F22、F24 | 家庭页请求序号、会话分页、额度返还 | 各 1h |

### Ignore for Now

- F21 重复删除申请（结果正确，仅重复通知）
- F23 导航栏窗口变化（需真机确认）
- F25 管理员 LLM URL（依赖管理员权限）
- F26 大文件拆分（随功能迭代渐进进行）
- F27 依赖替换（先升级间接驱动）
- F28、F29 默认值与文档说明

## 14. Quick Wins

| 编号 | 改动 | 价值 | 估时 |
|---|---|---|---|
| F02 | `config.Load` 生产环境对 `replace-with*`/默认值/短密钥 `log.Fatal` | 消除完全接管的配置风险 | 1h |
| F13 | `Order("id DESC").Limit(200)` 后反转 | 修复长会话"消息消失" | 30min |
| F14（部分） | `pageParams` 对 >100 截断为 100 | 避免调用方悄悄只拿到 20 条 | 10min |
| F12 | 批量分类复用 40 字符规则并检查 `.Error` | 修复 UI 撑破与假成功 | 30min |
| F04 | 先原子递增 `attempts` 再比较 | 恢复 5 次上限 | 1h |
| F15 | `onHide` 不取消提炼，提炼中禁用选图 | 避免次数被白扣 | 1h |
| F21 | 在 `lockFamily` 事务内查重创建 | 消除重复通知 | 30min |

## 15. Long-term Refactor Plan

**1. 分层准入（替代全站硬上限）**
- Motivation：F01、F05、F06、F07 都是"全站共享、按进程、fail-closed"的上限。
- Approach：Nginx 负责按 IP/连接的粗限流与请求体缓冲；应用层按路由类别（普通读、写、长连接、重任务、图片回源）分池，槽满先排队再拒绝，指标化拒绝率；重任务按内存预算准入。
- Risk：参数需要结合真实流量调优；多副本时需要明确"每副本容量"。
- Testing strategy：基于 httptest 的并发准入测试 + 预发环境压测（`hey`/`k6`），观察 p99 与拒绝率。

**2. 列表与选择器统一走服务端检索**
- Motivation：F13、F14、F22 都是"只取第一页并当成全集"。
- Approach：统一分页/游标契约（`items`、`next_cursor`、`has_more`），Web 与小程序各提供一个通用选择器模块，搜索与分类筛选下推到服务端。
- Risk：旧客户端兼容（保留 `page`/`pageSize`）。
- Testing strategy：以 600 道菜、500 条消息的大数据集做契约测试与 UI 冒烟。

**3. 数据生命周期与异步任务**
- Motivation：F08、F09、F10 都是随用户数/时间线性增长的同步操作。
- Approach：广播通知改为拉模式；导出与发布通知改为后台任务（`task_claims` 记录进度）；为事件类表设置保留期与月度汇总。
- Risk：数据迁移需要一次性回填与停写窗口。
- Testing strategy：迁移脚本在生产副本上演练，断言行数与抽样一致。
