# arre食谱推荐小助手 开发指南

## 环境要求

- Go 1.25+
- Node.js 20.19+ 或 22.12+（Vite 8 / jsdom 30）

## 后端开发

```bash
cd backend
go run cmd/server/main.go
```

默认运行在 `http://localhost:8080`。

## 前端开发

```bash
cd frontend
npm ci
npm run dev
```

Vite 开发服务器默认 `http://localhost:5173`，自动代理 API 到后端。

## 构建发布

### Windows

```bat
build_windows.bat
```

输出到 `dist-win/`，运行 `dist-win\ninimenu.exe`。

### Linux

```bat
build_linux.bat
```

输出 `ninimenu-linux-amd64.tar.gz`，部署：

```bash
tar -xzf ninimenu-linux-amd64.tar.gz -C /opt/ninimenu
chmod +x ninimenu
./ninimenu
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_ENV` | `development` | `production` / `release` 启用生产配置 |
| `APP_VERSION` | 空 | 语义版本 `X.Y.Z`（用户看到 `vX.Y.Z`）；由 `scripts/next-version.sh` 根据改动类型递增 |
| `APP_RELEASE_NOTES` | 空 | 面向用户的更新摘要，多条用“；”分隔；为空时使用通用更新文案 |
| `PORT` | `8080` | 服务端口 |
| `GOMEMLIMIT` | Compose：`256MiB` | Go 运行时内存软目标；非容器运行需在启动进程前导出，不包含本地 ASR 子进程、数据库或对象存储，也不是 RSS 硬上限 |
| `ADMIN_EMAIL` | 空 | 初始管理员邮箱；旧单用户数据迁移给该管理员 |
| `ADMIN_USERNAME` | `admin` | 初始管理员用户名 |
| `ADMIN_PASSWORD` | 仅开发有默认值 | 仅用于首次创建管理员和密码登录兜底；生产环境必须显式设置强密码 |
| `JWT_SECRET` | 开发用固定值 | JWT 签名密钥；生产环境必须设置至少 32 字节随机值 |
| `ALLOW_REGISTER` | `true` | 是否允许新邮箱注册 |
| `JWT_EXPIRE` | `720h` | 登录令牌有效期 |
| `DB_DRIVER` | `sqlite` | 本地用 SQLite；生产为 `mysql` |
| `MYSQL_DSN` | 空 | 生产 MySQL DSN，保存在受保护的部署配置中 |
| `DB_PATH` | `data/ninimenu.db` | SQLite 数据库路径 |
| `UPLOAD_DIR` | `uploads` | 本地上传目录；配置 S3 后使用对象存储 |
| `MAX_UPLOAD_SIZE_MB` | `5` | 图片上传大小上限 |
| `COMPRESS_MAX_DIM` / `JPEG_QUALITY` | `1200` / `85` | 图片压缩尺寸和 JPG 质量 |
| `SMTP_HOST` / `SMTP_PORT` | `smtp.qq.com` / `465` | 验证码邮件 SMTP 服务 |
| `SMTP_USER` / `SMTP_PASSWORD` | 空 | 邮箱账号与 SMTP 授权码；开发环境未配置时验证码写入后端日志 |
| `EMAIL_DOMAINS` | 空 | 邮箱域名白名单，逗号分隔 |
| `WECHAT_APPID` / `WECHAT_SECRET` | 空 | 小程序服务端登录配置；Secret 只放服务端环境变量 |
| `S3_ENDPOINT` / `S3_BUCKET` | `http://127.0.0.1:3900` / `cook-uploads` | S3 兼容对象存储；生产环境使用博客 Garage 的独立 bucket |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | 空 | 容器内的对象存储访问凭据；Compose 从 `.env` 的 `COOK_S3_ACCESS_KEY` / `COOK_S3_SECRET_KEY` 注入 |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | DeepSeek 默认地址 / 空 / `deepseek-chat` | 可选的站内 AI 模型服务 |
| `AGENT_RATE_LIMIT` | `120` | 外部 Agent 每账号每分钟请求上限，多个令牌共用；非正数回落 120，上限 600；与站内助手每日次数分开 |

可在项目根目录创建 `.env`。生产部署请按 `.env.example` 配置，具体 Compose、QQ 邮箱、Garage 和反向代理步骤见 [部署指南](docs/deployment.md)。`APP_PASSWORD` 与全站 `AGENT_TOKEN` 已不再使用；Agent 访问凭据由用户在「我的 → AI 连接」单独创建。

## 上传图片压缩机制

手机上传的截图和照片通常是体积很大的 PNG 格式。系统在上传时自动处理：

1. **原图备份** — 本地存储模式保存到 `BACKUP_DIR/u/<用户ID>/YYYY/MM/DD/`（`BACKUP_DIR` 默认 `uploads_backup`）；S3 模式只保存压缩结果
2. **压缩转换** — EXIF 方向修正 + 缩放（不超过 `COMPRESS_MAX_DIM`）+ 转 JPG（`JPEG_QUALITY` 质量）
3. **返回压缩 URL** — 前端拿到的是压缩后的 JPG 路径
4. **容错回退** — 普通压缩失败时使用原图；超过像素限制的图片直接拒绝，不回退保存

上传在读取文件前取得处理位置：单应用进程同时处理 1 张图片，最多 4 个请求等待、每个最多 5 秒；超出时返回 `429` 和重试提示。使用流式 multipart，只保留一份文件缓冲，不把整个表单载入内存或写入临时文件。单请求只接收一个 `image` 文件，文件仍受 `MAX_UPLOAD_SIZE_MB` 限制，表单开销最多额外 64 KiB、最多 8 个部分。文件或请求超限返回 `413`。

完整解码前检查图片头：最多 2400 万像素、单边不超过 16384 像素；超限返回 `400`，客户端需缩小图片再上传。缩放后复用 NRGBA 像素缓冲铺白底并编码，保持 EXIF 方向修正和 Lanczos 缩放。

PNG 截图转 JPG quality=85 通常减少 70-90% 体积，视觉几乎无损。

相关代码：
- 压缩核心：`backend/internal/imaging/compress.go`
- 上传处理：`backend/internal/handlers/upload.go`
- 配置项：`backend/internal/config/config.go`

上传失败时前端会展示后端返回的具体错误信息（如"图片大小不能超过5MB"），而非通用提示。

小程序端还必须在微信公众平台配置 request、uploadFile 合法域名和隐私保护指引（见 [微信小程序指南](docs/miniprogram.md)）；代码无法绕过这两个平台开关。

## 运行内存

周菜单只保留数据库中的 `week_plan_cache`，不再按用户永久保留进程内副本；每次读取增加一次用户设置查询，仍复用当周菜单并刷新当前收藏状态。

Compose 默认给 Go 运行时设置 `GOMEMLIMIT=256MiB`，使其在接近目标时更积极回收内存。该配置不预分配 256 MiB，也不限制 Python 语音识别或整个容器；不要据此将容器硬限制设为 256 MiB。本地 ASR 仍需要至少 768 MiB 可用余量才能启动。进一步降低 Go 目标前，应检查 GC CPU 和请求延迟。

局部内存分配对比可运行 `go test ./internal/imaging ./internal/handlers -run '^$' -bench 'Benchmark(CompressJPEG|ReadUploadImage)' -benchmem`。`B/op` 是每次操作累计分配的字节数，不是应用 RSS 峰值；整机容量仍需在 Linux 部署环境结合实际并发测量。

## 品牌资源

Web 和小程序的名称统一为「arre食谱推荐小助手」。技术标识（Go module、数据库、容器、缓存键）保留 `ninimenu`，避免破坏兼容性。

- Web 登录标记：`frontend/public/chef-mark.svg`。
- 小程序登录标记：`miniprogram/assets/chef-mark.svg`。
- Web PWA 图标：`frontend/public/32.png` 至 `512.png` 及 `manifest.json`。
- 本仓库当前没有历史文档中提到的 Python 图片生成脚本与 `pyproject.toml`；替换素材时直接更新上述资源。
- Go 图标处理工具源码在 `backend/cmd/imgtool/main.go`，可用 `go run ./cmd/imgtool -help` 查看用法。

## Go 图片处理依赖

| 包 | 用途 |
|----|------|
| `github.com/disintegration/imaging` | 图片缩放、EXIF 方向修正、Lanczos 重采样 |
| `golang.org/x/image/webp` | WebP 格式解码（间接依赖，用于上传兼容） |
| `image/jpeg`（标准库） | JPG 编码 |
| `image/draw`（标准库） | 透明图层合成白底 |

## 项目结构

```
arre-cook/
├── backend/                    # Go 后端
│   ├── cmd/
│   │   ├── server/main.go      # 服务入口
│   │   └── imgtool/main.go     # 图片处理工具源码（白边去除 & 多尺寸图标生成）
│   ├── internal/
│   │   ├── config/             # 环境变量配置
│   │   ├── database/           # 数据库初始化 & 种子数据
│   │   ├── handlers/           # HTTP 处理器
│   │   ├── imaging/            # 图片压缩处理（EXIF 修正 + 缩放 + JPG 编码）
│   │   ├── middleware/         # CORS, JWT, 日志中间件
│   │   ├── models/             # 数据模型
│   │   ├── routes/             # 路由注册
│   │   ├── services/           # 业务逻辑（推荐算法, 成就引擎, 周计划）
│   │   ├── dishes/             # 菜品种子数据包
│   │   ├── achievements/       # 成就目录
│   │   └── utils/              # 工具函数
│   ├── data/                   # SQLite 数据库
│   ├── uploads/                # 用户上传图片（压缩后的 JPG）
│   ├── uploads_backup/         # 用户上传原图备份
│   └── static/                 # 前端构建产物
├── frontend/                   # React 前端
│   ├── src/
│   │   ├── pages/              # 页面组件
│   │   ├── components/         # 通用组件
│   │   ├── layouts/            # 布局组件
│   │   ├── store/              # Zustand 状态
│   │   ├── types/              # TypeScript 类型
│   │   └── api/                # API 封装（含上传错误信息提取）
│   └── public/                 # 静态资源 & PWA 图标
├── miniprogram/                # 原生微信小程序页面、组件、工具
├── scripts/                    # 版本计算、客户端回归测试
├── build_windows.bat           # Windows 构建脚本
├── build_linux.bat             # Linux 构建脚本
└── .env                        # 本地环境变量
```

## API 概览

### 公开接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/app-info`、`/api/auth/options` | 应用名称、登录能力 |
| POST | `/api/auth/email/code`、`/api/auth/email/login` | 邮箱验证码申请与登录 |
| POST | `/api/auth/login`、`/api/auth/wechat` | 密码、微信登录 |
| GET | `/healthz` | 容器健康、存储、应用版本 |

### 用户接口（需用户 JWT）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/PUT | `/api/me`、`/api/me/preferences` | 当前用户资料与偏好 |
| GET/POST | `/api/dishes` | 查询可见菜谱、创建私房菜；管理员可显式创建公共菜谱 |
| GET | `/api/assistant/video-recipe/status` | 视频提炼与语音转写可用状态、共享 AI 额度 |
| POST | `/api/assistant/video-recipe` | SSE 视频 / 字幕提炼，只返回草稿；边界与配置见 [视频提炼](docs/video-recipes.md) |
| GET/PUT/DELETE | `/api/dishes/:id` | 详情及按作者/家庭权限校验的修改、删除 |
| GET | `/api/dishes/category-counts?scope=mine` | 本人个人菜谱分类计数 |
| GET/POST | `/api/records` | 查询与创建记录；查询支持 `from`、`to`、分页 |
| PUT/DELETE | `/api/records/:id` | 更新或删除自己的记录 |
| GET | `/api/favorites` | 收藏列表 |
| POST/DELETE | `/api/favorites/:dishId` | 收藏/取消收藏 |
| GET/POST | `/api/notifications`、`/api/notifications/:id/read` | 读取站内信 / 标记已读 |
| POST/DELETE | `/api/upload/image` | 上传、删除本人图片 |
| GET/PUT | `/api/settings` | 当前用户设置 |
| GET | `/api/week-plan`、`/api/shopping-list`、`/api/profile`、`/api/stats` | 计划、采购、画像、统计 |

ID 路由参数必须为正整数。菜谱 JSON 数组字段在应用修改前校验，错误输入不会覆盖旧内容。菜谱编辑权限以详情返回的 `access.can_edit` 为准；前端按钮隐藏不能替代后端授权。

### 管理接口（需管理员 JWT）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/PUT | `/api/admin/settings` | 站点设置，与个人设置区分 |
| GET | `/api/admin/dashboard`、`/api/admin/users` | 后台概览、用户列表 |
| PUT | `/api/admin/users/:id` | 管理账号 |
| POST | `/api/admin/notifications` | 站内通知 |
| POST/PUT/DELETE | `/api/quotes`、`/api/achievements`（修改/删除带 `:id`） | 语录、成就定义 |

完整路由以 `backend/internal/routes/routes.go` 为准。

### 智能体开放接口（需 `X-Agent-Token`）

食谱站把菜单、用餐记录、收藏、评价、行为事件、口味画像与推荐引擎全部开放给外部智能体，前缀 `/api/agent/*`。完整说明见 [docs/agent-api.md](docs/agent-api.md)，或直接请求 `GET /api/agent/capabilities` 获取自描述清单。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/agent/capabilities` | 能力清单与枚举值 |
| GET | `/api/agent/dishes`、`/api/agent/dishes/:id` | 菜品查询（含食材筛选、近期去重）与详情统计 |
| GET | `/api/agent/profile` | 口味画像 |
| POST | `/api/agent/recommend` | 推荐引擎 |
| GET/POST/DELETE | `/api/agent/records` | 用餐记录读写 |
| GET/POST/DELETE | `/api/agent/favorites` | 收藏读写 |
| GET/POST | `/api/agent/behavior` | 行为事件流读写 |
| GET | `/api/agent/day-ratings`、`/stats`、`/week-plan`、`/shopping-list`、`/settings`、`/export` | 评价、统计、周计划、买菜清单、设置、一次性导出 |

## 菜单范围

种子菜谱只保留南方菜系、以辣味为主，共 100 道：川菜 30、湘菜 30、贵州菜 18、云南菜 12、粤菜 10（`backend/internal/dishes/pack_*.go`，由 `catalog_test.go` 约束数量与菜系）。启动时会把历史数据库中已不在菜单里的旧种子菜品软删除（用户自建菜品不受影响）。

## 智能体接入

配套的「食谱推荐官」智能体在 `ai-agent-scaffold-lite` 工程中（`docs/dev-ops/recipe-agent.md`），通过用户创建的 Agent 令牌调用食谱工具。站内 `/assistant` 提供按条件选菜和站内对话入口，`/assistant/chat` 统一使用服务端模型、内容审核与额度；Web 页面已移除外部 iframe 和嵌入地址控件，历史 `agent_embed_url` 字段仅保留兼容。

站内模型接口只接受用户登录态。管理员在设置中调整每账号每日上限（0–20，默认 20）和全站每日预算（0–10000，默认 200）；北京时间 00:00 重置，数据保存在数据库，删除对话和部署不会清空次数。同账号同时 1 请求、全站同时 4 请求。审核、错误码、SSE 协议及防护边界见 [助手安全与额度](docs/assistant-safety.md)。

## 客户端状态与回归

- UI 规范见 [docs/ui-guide.md](docs/ui-guide.md)。Web 页面对话框复用 `use-dialog` 和 `AnimatedBottomSheet`，小程序复用 `sheet` 与 `sheet-tabs`。
- 新增的草稿功能使用账号 + 编辑模式 + 菜谱 ID 组成存储键，离开时立即写入；只存表单和上传后的 URL，不存图片二进制和令牌。
- 格式转换集中在两端的 `recipe-text` 工具，未修改的食材、步骤及图片元数据必须保留。
- 执行 `cd frontend && npm test` 运行客户端回归；执行 `npm run lint`、`npm run build` 做静态检查。后端运行 `go test ./...`、`go vet ./...`。
- 发布先提交实现，再执行 `./scripts/next-version.sh`，将结果写入并提交 `VERSION`。新功能递增 minor，修复递增 patch，不兼容变更递增 major。
