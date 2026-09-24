# NiniMenu 开发指南

## 环境要求

- Go 1.25+
- Node.js 20+
- Python 3.10+（图片工具脚本）
- uv（Python 包管理）

## 后端开发

```bash
cd backend
go run cmd/server/main.go
```

默认运行在 `http://localhost:8080`。

## 前端开发

```bash
cd frontend
npm install
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
| `PORT` | `8080` | 服务端口 |
| `ADMIN_EMAIL` | 空 | 初始管理员邮箱；旧单用户数据迁移给该管理员 |
| `ADMIN_USERNAME` | `admin` | 初始管理员用户名 |
| `ADMIN_PASSWORD` | `nini123` | 仅用于首次创建管理员和密码登录兜底；生产环境必须显式设置强密码 |
| `JWT_SECRET` | 开发用固定值 | JWT 签名密钥；生产环境必须设置至少 32 字节随机值 |
| `ALLOW_REGISTER` | `true` | 是否允许新邮箱注册 |
| `JWT_EXPIRE` | `720h` | 登录令牌有效期 |
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
| `AGENT_RATE_LIMIT` | `120` | 每个 Agent 令牌每分钟请求上限 |

可在项目根目录创建 `.env`。生产部署请按 `.env.example` 配置，具体 Compose、QQ 邮箱、Garage 和反向代理步骤见 [部署指南](docs/deployment.md)。`APP_PASSWORD` 与全站 `AGENT_TOKEN` 已不再使用；Agent 访问凭据由用户在「我的 → AI 连接」单独创建。

## 上传图片压缩机制

手机上传的截图和照片通常是体积很大的 PNG 格式。系统在上传时自动处理：

1. **原图备份** — 原始文件保存到 `BACKUP_DIR`（默认 `uploads_backup/YYYY/MM/DD/`）
2. **压缩转换** — EXIF 方向修正 + 缩放（不超过 `COMPRESS_MAX_DIM`）+ 转 JPG（`JPEG_QUALITY` 质量）
3. **返回压缩 URL** — 前端拿到的是压缩后的 JPG 路径
4. **容错回退** — 如果压缩失败，自动使用原图

PNG 截图转 JPG quality=85 通常减少 70-90% 体积，视觉几乎无损。

相关代码：
- 压缩核心：`backend/internal/imaging/compress.go`
- 上传处理：`backend/internal/handlers/upload.go`
- 配置项：`backend/internal/config/config.go`

上传失败时前端会展示后端返回的具体错误信息（如"图片大小不能超过5MB"），而非通用提示。

## 图片工具脚本

脚本依赖 `openai` 和 `pillow`，安装：

```bash
uv sync
```

### 单张生成

```bash
uv run python scripts/generate_dish_image.py --prompt "菜品为番茄炒蛋 将宽高比设为 1:1" --output tomato_egg.png
```

### 批量生成

编辑 `dish_images.txt`，格式为 `菜品名 输出路径`，然后：

```bash
uv run python scripts/generate_dish_images_batch.py --input dish_images.txt --output-root output
```

支持 `--submit-only`（仅提交请求）和 `--recover-history`（从历史记录下载）模式。

### 图片压缩（离线批量处理）

```bash
uv run python scripts/compress_upload_images.py --input-dir output/uploads --auto-quality
```

> 这是离线批量脚本，用于压缩预生成的菜品图片。运行时上传压缩由 Go 后端自动完成。

## Go 图片处理依赖

| 包 | 用途 |
|----|------|
| `github.com/disintegration/imaging` | 图片缩放、EXIF 方向修正、Lanczos 重采样 |
| `golang.org/x/image/webp` | WebP 格式解码（间接依赖，用于上传兼容） |
| `image/jpeg`（标准库） | JPG 编码 |
| `image/draw`（标准库） | 透明图层合成白底 |

## 项目结构

```
NiniMenu/
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
│   ├── imgtool.exe             # 图片白边去除 & 多尺寸图标生成工具
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
├── scripts/                    # Python 图片工具脚本
├── build_windows.bat           # Windows 构建脚本
├── build_linux.bat             # Linux 构建脚本
├── pyproject.toml              # Python 脚本依赖
└── .env                        # 本地环境变量
```

## API 概览

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/app/login` | 应用端密码登录 |
| POST | `/api/admin/login` | 管理端密码登录 |
| GET | `/api/app-info` | 应用信息（名称等） |

### 应用接口（需 App Token 或 JWT）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/dishes` | 菜品列表 |
| GET | `/api/dishes/:id` | 菜品详情 |
| GET | `/api/dishes/:id/records` | 菜品用餐记录 |
| GET/POST/PUT/DELETE | `/api/records` | 用餐记录 CRUD |
| POST | `/api/records/batch` | 批量创建记录 |
| GET/POST | `/api/favorites/:dishId` | 收藏/取消收藏 |
| POST | `/api/pick/lunch\|dinner\|mood\|tomorrow\|blind-box` | 各类推荐（午/晚餐与心情推荐已接入推荐引擎） |
| POST | `/api/pick/smart` | 智能推荐：口味画像 + 约束打分，返回带理由的结果 |
| GET | `/api/profile` | 口味画像 |
| POST | `/api/behavior` | 前端行为埋点（view / accept / reject …） |
| GET/POST | `/api/day-rating` | 每日评价 |
| GET | `/api/photo-wall` | 照片墙 |
| GET/POST | `/api/shopping-list` | 购物清单 |
| GET | `/api/week-plan` | 周计划 |
| GET | `/api/achievements` | 成就列表 |
| GET | `/api/stats` | 统计数据 |

### 管理接口（需 JWT）

| 方法 | 路径 | 说明 |
|------|------|------|
| CRUD | `/api/dishes` | 菜品管理 |
| POST | `/api/dishes/batch-*` | 批量操作 |
| POST | `/api/upload/image` | 图片上传（自动备份+压缩） |
| DELETE | `/api/upload/image` | 图片删除 |
| CRUD | `/api/quotes` | 语录管理 |
| CRUD | `/api/achievements` | 成就管理 |
| PUT | `/api/settings` | 系统设置 |
| GET | `/api/admin/dashboard` | 仪表盘 |

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

配套的「食谱推荐官」智能体在 `ai-agent-scaffold-lite` 工程中（`docs/dev-ops/recipe-agent.md`）。管理后台 → 设置 → 「AI 推荐官」填入智能体嵌入页地址后，站内 `/assistant` 页面会内嵌对话组件；该页同时展示口味画像并可按条件调用推荐引擎。
