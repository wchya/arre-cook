# 部署

## 生产配置

### MySQL 数据库

食谱站使用博客服务器已有的 MySQL 8 实例，但使用独立的 `ninimenu` 数据库和
`ninimenu` 用户，不读取或写入博客的 `blog` schema。先在博客 MySQL 容器中创建
数据库和最小权限账号，再把账号写入食谱站 `.env`：

```sql
CREATE DATABASE ninimenu CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER 'ninimenu'@'%' IDENTIFIED BY '生成一段随机密码';
GRANT ALL PRIVILEGES ON ninimenu.* TO 'ninimenu'@'%';
FLUSH PRIVILEGES;
```

```dotenv
DB_DRIVER=mysql
MYSQL_HOST=mysql
MYSQL_PORT=3306
MYSQL_DATABASE=ninimenu
MYSQL_USER=ninimenu
MYSQL_PASSWORD=同一段随机密码
```

切换前先用 SQLite 在线备份，然后停止食谱容器，在共享 Docker 网络中运行迁移：

```sh
sqlite3 /var/lib/docker/volumes/arre-cook_ninimenu-data/_data/ninimenu.db \
  ".backup '/home/ubuntu/backups/arre-cook/ninimenu-before-mysql-$(date +%Y%m%d-%H%M%S).db'"
MYSQL_DSN='ninimenu:密码@tcp(mysql:3306)/ninimenu?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai'
docker run --rm --network web-arrebyte_default \
  -v arre-cook_ninimenu-data:/source:ro \
  -e MYSQL_DSN="$MYSQL_DSN" \
  --entrypoint /app/dbmigrate arre-cook/ninimenu:latest \
  -source /source/ninimenu.db -target-dsn "$MYSQL_DSN"
```

`backend/cmd/dbmigrate` 是独立迁移程序，会先创建目标表，再按外键依赖顺序保留原始主键复制所有用户、家庭、饮食、通知和 Agent 数据。生产切换前应在临时 schema 先演练，并比较源库和目标库各表行数。

生产 `.env` 位于服务器 `/home/ubuntu/arre-cook/.env`，包含管理员、SMTP 和 Garage 配置；当前权限为 `600`。`ADMIN_EMAIL` 是初始管理员账号，也是旧版单用户数据迁移后的归属账号。不要把 `.env` 或小程序私钥提交到 Git，也不要在日志或文档中记录密钥值。

博客服务器 `/home/ubuntu/web-arrebyte/.env` 已有 QQ 邮箱的 `MAIL_HOST`、`MAIL_PORT`、`MAIL_USERNAME` 和 `MAIL_PASSWORD`。在本仓库根目录运行 Compose 时，让它读取博客和应用两份环境文件；博客 SMTP 变量会传给菜谱容器，博客数据库等变量不会传入。菜谱的 Garage 凭据必须填写 `COOK_S3_ACCESS_KEY` / `COOK_S3_SECRET_KEY`，Compose 不会复用博客的 `S3_ACCESS_KEY`。博客当前使用 `smtp.qq.com:587`（STARTTLS），`.env.example` 已对齐：

```sh
APP_VERSION="$(./scripts/next-version.sh)" \
APP_RELEASE_NOTES="优化站内信详情查看体验；修复头像选择无响应问题；新增我的私房菜入口" \
docker compose --env-file ../web-arrebyte/.env --env-file .env up -d --build
```

服务器已存在 `web-arrebyte_default` 网络，Garage 在其中有 `garage` 服务别名。生产菜谱容器已接入该网络。每次部署运行 `./scripts/next-version.sh`，只根据上次更新 `VERSION` 之后的提交计算版本：`fix` 或普通改动递增 patch，`feat` 递增 minor，提交标题中的 `!:` 或正文中的 `BREAKING CHANGE:` / `BREAKING-CHANGE:` 递增 major。没有新提交时保持当前版本，避免重复通知。将发布版本写入 `VERSION` 并提交，再把它传给 `APP_VERSION`，用户会看到 `vX.Y.Z`。摘要为空时使用“本次更新了一些内容，并修复了一些 bug。”。

## Garage 单机运行与备份

截至 2026-09-24，博客 Garage 运行在 `arre-tx` 单机，`replication_factor = 1`，当前布局只有一个健康节点；已弃用的 `cs` 主机已从布局移除，不要重新加入。博客与菜谱服务共用该 Garage 实例，通过 `web-arrebyte_default` 网络访问。

单节点没有跨主机副本。Garage 主机或数据卷不可用时，图片存储也会中断。切换前的 meta/data、配置和密钥备份位于服务器 `/home/ubuntu/backups/garage-single-node-20260924/`，所有备份文件权限为 `600`。恢复时必须使用同一时间点的 meta 与 data 备份，并先保留当前数据副本；不要把备份的 Garage 密钥复制进仓库或公开日志。

例行备份需安排短暂维护窗口，因为停止 Garage 时对象存储不可读写。先停止服务，再归档两个 Docker 卷和 Garage 配置，完成后启动服务并限制备份文件权限。当前线上卷路径为：

```sh
set -eu
umask 077
cd /home/ubuntu/web-arrebyte
backup_path="/home/ubuntu/backups/garage-single-node-$(date +%Y%m%d-%H%M%S)"
install -d -m 700 "$backup_path"
docker compose --profile garage stop garage
trap 'docker compose --profile garage start garage' EXIT
tar -C /var/lib/docker/volumes/web-arrebyte_garage-meta/_data -czf "$backup_path/garage-meta.tar.gz" .
tar -C /var/lib/docker/volumes/web-arrebyte_garage-data/_data -czf "$backup_path/garage-data.tar.gz" .
config_path="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/etc/garage.toml"}}{{.Source}}{{end}}{{end}}' web-arrebyte-garage-1)"
cp "$config_path" "$backup_path/garage.toml"
chmod 600 "$backup_path"/*
docker compose --profile garage start garage
trap - EXIT
```

确认 Garage 健康后再结束维护。备份目录包含存储配置和密钥材料，应限制访问并纳入异机备份；定期在隔离环境演练恢复。

## Garage 上传密钥

博客 Garage 有 `blog-uploads` bucket 和博客专用密钥。菜谱应用使用独立的 `cook-uploads` bucket 与专用密钥，该密钥只对该 bucket 有读写权限。生产环境已创建并配置菜谱专用密钥；以下命令只用于新环境初始化，不要在线上重复创建：

```sh
cd /home/ubuntu/web-arrebyte
docker compose --profile garage exec garage /garage bucket create cook-uploads
docker compose --profile garage exec garage /garage key create arre-cook
docker compose --profile garage exec garage /garage bucket allow --read --write cook-uploads --key arre-cook
```

把创建密钥时输出的 access key 和 secret 写进菜谱应用服务器上的 `.env`：`COOK_S3_ACCESS_KEY`、`COOK_S3_SECRET_KEY`，不要使用博客站已有的 S3 凭据。上传对象放在 `cook/u/<user-id>/...` 前缀；服务端会校验删除权限。菜谱应用自身的 `/uploads/` 路由负责读取历史本地图片和 Garage 图片，无需改博客站的公开图片路由。

生产验证已通过：管理员登录后上传图片返回 `storage=s3`；应用 `/uploads/` 回读返回 `200 image/jpeg`；删除后回读返回 `404`。健康检查 `/healthz` 同时报告 `storage=s3`。

升级新版前先备份 MySQL `ninimenu` 数据库。迁移前的旧 SQLite 源库位于 `arre-cook_ninimenu-data` 卷，使用 WAL，不能只复制 `ninimenu.db` 而漏掉 `-wal` 文件。此次升级前已执行在线备份，并在隔离副本完成迁移演练；后续如需回滚或重建仍应先备份：

```sh
mkdir -p /home/ubuntu/backups/arre-cook
backup_path="/home/ubuntu/backups/arre-cook/pre-multiuser-$(date +%Y%m%d-%H%M%S).db"
sqlite3 /var/lib/docker/volumes/arre-cook_ninimenu-data/_data/ninimenu.db ".backup '$backup_path'"
sqlite3 "$backup_path" 'PRAGMA integrity_check;'
chmod 600 "$backup_path" /home/ubuntu/arre-cook/.env
```

完整性检查应输出 `ok`。启动时会自动把旧版个人数据归到 `ADMIN_EMAIL` 对应账号；迁移完成后，先检查管理员账号的记录、收藏、偏好和上传图片，再开放新用户注册。

## 反向代理

`cook.arrebyte.top` 的 HTTPS 虚拟主机已接到博客 Docker 网络内的 `ninimenu:8080`，公网 `/healthz` 已验证返回 `UP` 和 `storage=s3`。API、H5 和 `/uploads/` 都由同一个 Go 服务处理。反向代理要传递原始 Host、客户端 IP 和 HTTPS scheme；生产环境的 `PUBLIC_URL`、`CORS_ORIGINS` 应与实际域名一致。

应用也绑定宿主机 `127.0.0.1:9925` 供本机检查：

```sh
curl -fsS http://127.0.0.1:9925/healthz
```

首次创建管理员之后，设置 `ALLOW_REGISTER=false` 可以关闭公开注册。邮箱验证码有服务端冷却和 IP 限流；SMTP 未配置时生产环境无法登录。

## 内存预算

Compose 默认注入 `GOMEMLIMIT=192MiB`、`GOMAXPROCS=2`，整个应用容器硬限制为 `APP_MEMORY_LIMIT=1g`、`APP_CPUS=2`，memory+swap 等于 memory。Go 软目标不涵盖 Python ASR、FFmpeg 或 tmpfs，容器限制涵盖这些进程；MySQL、Garage 和其他服务需要另外核算。使用本地 ASR 时仍需至少 768 MiB 可用余量，单任务 RSS 上限 512 MiB。已有 `.env` 若显式设置旧的 `256MiB` 会覆盖默认值，发布时需检查。

单应用进程同时处理 1 张上传图片，最多 4 个请求等待 5 秒；图片解码与 ASR 进一步共用一个重任务槽，忙碌返回 `429` 或转写忙碌状态。图片完整解码前检查 2400 万像素和单边 16384 像素上限。普通 API/MCP 最多 16 个请求，密码计算最多 2 个，菜库/导出读取最多 2 个，公共图片回源最多 8 个。普通 JSON 上限 64 KiB，菜谱/Agent/MCP 上限 256 KiB。周菜单缓存只保存 ID，并重新读取当前可见内容。具体规则见 [开发指南](../README_DEV.md#运行内存)。这些配置需发布并重建容器后生效；4 核、3.32 GiB、无 swap、13 容器混部的组合负载仍需验证，不能承诺固定峰值或 QPS。

## AI 与智能体

v0.10.0 增加持久的每日额度与并发租约表，启动时自动迁移。管理员可配置每账号每日 0–20 次（默认 20）和全站每日 0–10000 次（默认 200）。上线前须备份 MySQL，保留旧镜像，并在升级后检查两个新表及 `/api/assistant/status` 返回的额度。完整规则见 [助手安全与额度](assistant-safety.md)。

视频提炼使用 `video_platform_budgets` 表保存平台读取预算，不迁移或重写菜谱数据。发布前须备份数据库并保留当前镜像。默认 `VIDEO_ASR_PROVIDER=remote` 且没有完整远端配置时只尝试字幕，提供粘贴文稿入口；不要将 CPA 文本模型密钥填作转写密钥。

抖音可选配置 `VIDEO_DOUYIN_COOKIE_FILE=/run/video-secrets/douyin-guest.json`。凭据由运维电脑上的 `scripts/douyin-guest-cookies.py` 使用独立游客浏览器生成，放入服务器 `secrets/video/douyin-guest.json`，文件属主 `1000:1000`、权限 `600`；Compose 只读挂载整个目录。服务器不安装浏览器或 yt-dlp。配置环境变量后重建容器，之后同目录原子替换凭据无需重启；凭据过期需运维刷新，冷却和请求预算不重置。完整步骤与资源边界见 [抖音游客凭据](video-recipes.md#抖音游客凭据)。该链路目前仅本机元数据验收通过，生产仍需少量验证，不能据此宣称所有抖音视频均可转写。

本地免费文字提取使用 `VIDEO_ASR_PROVIDER=local`，在现有应用容器内按需启动 SenseVoice 子进程，不部署其他服务。发布前运行 `python3 backend/video_asr/download_models.py --destination models/video-asr`，确认三个模型文件的大小和 SHA-256 符合锁定清单。网络不通时可从可联网机器传入已验证文件。模型目录只读挂载，不放入 Git；工作目录是 64 MiB tmpfs。Linux amd64 / arm64 的宿主需要支持 Landlock 和默认容器 seccomp，不能通过关闭沙箱让转写启用。

本地模式启动任务至少需要 768 MiB 可用内存，单进程同时只运行一个转写任务，RSS 超过 512 MiB 则终止。当前小内存服务器不应直接增大并发或复制 ASR 实例。首次状态请求会校验依赖、模型和沙箱；发布后检查 `/api/assistant/video-recipe/status` 的 `asr_enabled=true`，再少量验证真实视频。配置与验收项见 [视频做法提炼](video-recipes.md)。回滚仅切换应用镜像和配置，保留模型文件与原数据库；本轮没有新增菜谱迁移。

生产环境优先使用线上 Hermes / DSH 已在使用的 CPA 配置。应用容器以只读方式挂载 DSH 的 `llm-override.json`，运行时读取其中的 `baseUrl`、`apiKey`、`model` 和 `completionsPath`，访问同一个 CPA OpenAI 兼容接口；CPA 密钥不会写入菜谱仓库、数据库或日志。服务器 `/home/ubuntu/arre-cook/.env` 应包含：

```sh
CPA_CONFIG_PATH=/home/ubuntu/ai-agent-scaffold-lite/docs/dev-ops/config/llm-override.json
LLM_CPA_CONFIG_PATH=/run/cpa/provider-config
LLM_CPA_BASE_URL=http://host.docker.internal:8317/v1
LLM_CPA_MODEL=cook/glm-5.3
```

线上 DSH 文件当前指向 `http://172.22.0.1:8317`、`v1/chat/completions` 和 `glm-5.3`；应用读取其中的 CPA 密钥，并用 `LLM_CPA_BASE_URL` 将容器内地址映射到宿主机 CPA 端口。`LLM_CPA_MODEL` 可覆盖该文件中的模型，未设置时仍使用文件中的模型。生产食谱应用使用 CPA 内独立别名 `cook/glm-5.3`，通过智谱订阅的原生 Chat Completions 入口转发，使 `reasoning_effort=low` 正确生效；上游订阅密钥仍只保存在 CPA。该别名通过 CPA 管理接口持久配置，保留既有模型路由。只有未配置 CPA 文件或 CPA 配置缺失时，才回退到管理员设置或 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`。未配置任何模型时，站内助手仍使用本地推荐引擎。

每位用户在「我的 → AI 连接」创建自己的访问令牌。外部 Agent 和 MCP 客户端必须使用该用户的令牌；不要配置旧的全站 `AGENT_TOKEN`。发给第三方的令牌可以修改名称/权限/有效期、撤销或轮换，并在调用记录中查看。`/api/agent/*` 与 `/mcp` 拒绝长期站内登录 JWT，只接受 `nm_` 个人令牌或短期 Agent session；Hermes、DSH 必须分别配置各自用户创建的令牌，不能共用。

站内 `/api/assistant/chat` 的凭证方向相反：只接受站内用户 JWT，拒绝 PAT/短期 Agent 令牌。所有站内对话经过食谱输入审核、写入操作审核和完整输出审核；新版本不再嵌入外部模型页面。审核器使用同一服务端提供商，只接受正常结束的 `ALLOW` / `BLOCK`；20 秒内无法完成或回复被截断时拒绝继续。上线后必须验证真实模型，不能仅凭 mock 测试确认审核可用。

生产 `PUBLIC_URL` / `CORS_ORIGINS` 应明确填写 `https://cook.arrebyte.top`，AI 入口不会接受通配符给予的外站信任。反向代理要覆盖伪造转发头；Compose 支持通过 `TRUSTED_PROXIES` 指定实际代理地址。账号级认证、改密、对话、视频和 Agent 速率额度已由数据库共享窗口预约，每日额度与租约也使用数据库；进程级入口保护和重任务槽仍只保护本副本。当前混部主机保持应用/本地 ASR 单副本。

### 审计修复的首次迁移与发布

2026-09-28 的修复状态及验证范围见 [逐项修复记录](audit-remediation-2026-09-28.md)。0.12.7（`8085433`）已从线上 0.12.5 升级完成，当前 `DB_AUTO_MIGRATE=false`，容器 1 GiB / 2 CPU、Go 192 MiB 和 110 秒停止等待已核对生效。执行时间、备份、回滚和小程序上传回执见 [发布记录](releases/0.12.7.md)。以下保留首次迁移的操作流程，后续发布应按当时的数据和代码重新核验。

1. 先完成实现与版本提交、构建镜像，保留旧镜像，并备份 MySQL、Garage 与本地上传目录。镜像后端固定 Go 1.25.14，`x/image` 为 0.45.0。
2. 在隔离环境验证原数据库副本迁移和恢复。新表包括 `upload_assets`、`upload_references`、`task_claims`、`request_windows`，成就新增用户/成就唯一键，迁移先合并重复授奖；菜数非法历史设置重置为 1。离线 `dbmigrate` 使用同一模型清单，包含家庭清单/删除申请，游标读取并逐表核对行数。
3. 首次建立对象账本需 bucket 的列举权限（ListObjectsV2）以及读写/删除权限；以每页 500 个对象扫描 S3、分批回填业务引用，并统计历史本地文件与备份。存储扫描最多 10 分钟，失败则迁移退出，不启用缺失账本的 API。历史模糊备份按 `archive` 保留并计入额度。若已有数据超额，不自动删除已引用图片，只拒绝新增。
4. 切入维护窗口、停止新请求，并留足现有 AI 请求的 90 秒完成时间后停止旧容器；执行一次迁移任务。MySQL 迁移用固定连接的 `GET_LOCK` 串行初始化；1 连接配置仅在迁移时临时借用第二个连接。完成后再启动 API 副本并设置 `DB_AUTO_MIGRATE=false`。

```sh
docker compose --env-file ../web-arrebyte/.env --env-file .env build ninimenu
# 完成备份、预发验证、切流和在途请求排空后：
docker compose --env-file ../web-arrebyte/.env --env-file .env stop ninimenu
docker compose --env-file ../web-arrebyte/.env --env-file .env run --rm --no-deps \
  -e DB_AUTO_MIGRATE=true -e DB_MIGRATE_ONLY=true ninimenu
DB_AUTO_MIGRATE=false docker compose --env-file ../web-arrebyte/.env --env-file .env up -d --no-build ninimenu
```

将 `DB_AUTO_MIGRATE=false` 持久写入部署 `.env`，每次需要 schema 变更时先运行上面的独立迁移。更新后的普通请求期限 15 秒，AI/Agent/MCP 为 90 秒，应用 Shutdown 等待 100 秒，Compose stop grace 为 110 秒；请求根 context 在排空结束后才取消。代理须先停止接流，避免停止期间持续重试旧实例。Nginx reload 返回时新 worker 未必已经接流，解除维护后应在有限时间内轮询公网预期版本，不能把重载后第一次 503 直接判为部署失败；回滚重建容器后也应 reload 以刷新上游地址，并确认公网恢复。

发布后核对模块版本、容器 memory/CPU/stop timeout、健康、登录与会话撤销、图片上传/引用保护、菜单、家庭清单和站内信；在 Linux 预发验证最大图片与 ASR/菜单/其他容器组合负载及执行中 SIGTERM。小程序开发版还要做 iOS 真机弹层检查。回滚到不维护图片账本的旧版本后，不可直接沿用旧的 `uploads_inventory_v1=done`：再次升级前应在停写和备份之后将该标记重置为待回填，重新核对引用及对象总量。

### 发布核验

1. 检查工作区和远端提交，先提交实现，再按版本脚本写入并提交 `VERSION`；根据实际改动整理用户可读的发布摘要。
2. 用 Netcatty MCP 在 `arre-tx` 备份独立的 `ninimenu` 数据库，备份权限 `600`；保存当前镜像的回滚 tag。
3. 拉取并核对发布 revision，传入同一 `APP_VERSION` / `APP_RELEASE_NOTES`，执行 Compose 构建并等待健康。
4. 检查本机与公网 `/healthz`、管理员登录、成就查询、图片回读、两个助手表、站内信版本与内容，以及启动后有无 SQL 1064。
5. 使用少量真实模型请求验证普通食谱可回答、无关任务被拒绝、客户端模型覆盖被拒绝和已用次数正确；检查卡片和状态事件，不输出凭证或原始上游错误。
6. 使用微信官方 `miniprogram-ci` 上传相同版本，保存上传回执。上传成功表示后台开发版本，仍需微信体验版真机检查和平台审核发布。

回滚时停止新容器并以记录的旧镜像和版本启动；新增额度表可保留，不要为了回滚删除个人数据。只有确认数据库迁移导致问题时，才在保留现场副本后恢复升级前备份。

站内信表随独立迁移任务更新；生产 API 启动保持 `DB_AUTO_MIGRATE=false`。系统更新、Agent 安全事件、家庭邀请和 AI 饮食建议会生成对应用户的通知；管理员可在「管理 → 设置 → 站内信通知」广播更新，消息按 `user_id` 独立保存并维护各自已读状态。

## 数据迁移检查

2026-09-24 家庭与饮食报告升级：上线前备份位于 `/home/ubuntu/backups/arre-cook/pre-family-health-4f94aab.db`（权限 `600`）；使用隔离副本运行新版自动迁移后，新增家庭与饮食日记表，原有用户/菜谱/用餐记录数仍为 `2/632/2`，`PRAGMA integrity_check` 为 `ok`。生产升级后再次确认相同数量和完整性，容器健康且 `/healthz` 报告 `storage=s3`。本次升级没有迁移个人记录到家庭空间。

备份数据库之后再升级。升级启动后至少检查：

- `ADMIN_EMAIL` 登录后能看到原账户的记录、收藏和评分；旧全局周计划缓存会清除并按用户重新生成。
- 新注册的第二个账号看不到管理员的个人记录、私房菜、建议和对话。
- 以只读 Agent 令牌调用写入工具会被拒绝；撤销令牌后立即失效。
- 上传图片的地址可通过 `/uploads/` 读取，普通用户无法删除别人的对象。

本地自动化验证：`cd backend && go test ./... && go vet ./...`，以及 `cd frontend && npm run build && npm run lint`。生产数据库迁移仍应先在副本上演练。
