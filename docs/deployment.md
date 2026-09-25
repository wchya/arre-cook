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
docker compose --env-file ../web-arrebyte/.env --env-file .env up -d --build
```

服务器已存在 `web-arrebyte_default` 网络，Garage 在其中有 `garage` 服务别名。生产菜谱容器已接入该网络，通过 `http://garage:3900` 访问对象存储。若博客项目名改变，在 `.env` 中调整 `BLOG_DOCKER_NETWORK`。Compose 文件或环境配置变更后使用 `docker compose --env-file ../web-arrebyte/.env --env-file .env up -d --build` 重建容器。

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

升级新版前先用 SQLite 在线备份旧库。线上旧库位于 `arre-cook_ninimenu-data` 卷，使用 WAL，不能只复制 `ninimenu.db` 而漏掉 `-wal` 文件。此次升级前已执行在线备份，并在隔离副本完成迁移演练；后续升级仍应按以下方式先备份：

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

## AI 与智能体

生产环境优先使用线上 Hermes / DSH 已在使用的 CPA 配置。应用容器以只读方式挂载 DSH 的 `llm-override.json`，运行时读取其中的 `baseUrl`、`apiKey`、`model` 和 `completionsPath`，访问同一个 CPA OpenAI 兼容接口；CPA 密钥不会写入菜谱仓库、数据库或日志。服务器 `/home/ubuntu/arre-cook/.env` 应包含：

```sh
CPA_CONFIG_PATH=/home/ubuntu/ai-agent-scaffold-lite/docs/dev-ops/config/llm-override.json
LLM_CPA_CONFIG_PATH=/run/cpa/provider-config
LLM_CPA_BASE_URL=http://host.docker.internal:8317/v1
```

线上 DSH 文件当前指向 `http://172.22.0.1:8317`、`v1/chat/completions` 和 `glm-5.3`；应用读取其中的密钥和模型，并用 `LLM_CPA_BASE_URL` 将容器内地址映射到宿主机 CPA 端口。Hermes 的配置用于交叉确认同一 CPA 地址和模型，生产应用当前以 DSH JSON 覆盖文件作为唯一来源。只有未配置 CPA 文件或 CPA 配置缺失时，才回退到管理员设置或 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`。未配置任何模型时，站内助手仍使用本地推荐引擎。

每位用户在「我的 → AI 连接」创建自己的访问令牌。外部 Agent 和 MCP 客户端必须使用该用户的令牌；不要配置旧的全站 `AGENT_TOKEN`。发给第三方的令牌可以修改名称/权限/有效期、撤销或轮换，并在调用记录中查看。`/api/agent/*` 与 `/mcp` 拒绝长期站内登录 JWT，只接受 `nm_` 个人令牌或短期 Agent session；Hermes、DSH 必须分别配置各自用户创建的令牌，不能共用。

站内信表会在启动时自动迁移。系统更新、Agent 安全事件、家庭邀请和 AI 饮食建议会生成对应用户的通知；管理员可在「管理 → 设置 → 站内信通知」广播更新，消息按 `user_id` 独立保存并维护各自已读状态。

## 数据迁移检查

2026-09-24 家庭与饮食报告升级：上线前备份位于 `/home/ubuntu/backups/arre-cook/pre-family-health-4f94aab.db`（权限 `600`）；使用隔离副本运行新版自动迁移后，新增家庭与饮食日记表，原有用户/菜谱/用餐记录数仍为 `2/632/2`，`PRAGMA integrity_check` 为 `ok`。生产升级后再次确认相同数量和完整性，容器健康且 `/healthz` 报告 `storage=s3`。本次升级没有迁移个人记录到家庭空间。

备份数据库之后再升级。升级启动后至少检查：

- `ADMIN_EMAIL` 登录后能看到原账户的记录、收藏和评分；旧全局周计划缓存会清除并按用户重新生成。
- 新注册的第二个账号看不到管理员的个人记录、私房菜、建议和对话。
- 以只读 Agent 令牌调用写入工具会被拒绝；撤销令牌后立即失效。
- 上传图片的地址可通过 `/uploads/` 读取，普通用户无法删除别人的对象。

本地自动化验证：`cd backend && go test ./... && go vet ./...`，以及 `cd frontend && npm run build && npm run lint`。生产数据库迁移仍应先在副本上演练。
