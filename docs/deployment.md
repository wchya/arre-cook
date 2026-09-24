# 部署

## 生产配置

服务器 `/home/ubuntu/arre-cook` 已有旧版 `.env`，应保留其中的 `ADMIN_PASSWORD` 和 `JWT_SECRET`，参照 `.env.example` 补齐 `ADMIN_EMAIL`、SMTP、Garage 等新配置，不要直接覆盖旧文件。`ADMIN_EMAIL` 是首次管理员账号，也是旧版单用户数据迁移后的归属账号。不要把 `.env` 或小程序私钥提交到 Git，并将 `.env` 权限设为 `600`。目前线上旧版 `.env` 权限为 `644`，升级前需要收紧。

博客服务器 `/home/ubuntu/web-arrebyte/.env` 已有 QQ 邮箱的 `MAIL_HOST`、`MAIL_PORT`、`MAIL_USERNAME` 和 `MAIL_PASSWORD`。在本仓库根目录运行 Compose 时，让它读取博客和应用两份环境文件；博客 SMTP 变量会传给菜谱容器，博客数据库等变量不会传入。菜谱的 Garage 凭据必须填写 `COOK_S3_ACCESS_KEY` / `COOK_S3_SECRET_KEY`，Compose 不会复用博客的 `S3_ACCESS_KEY`。博客当前使用 `smtp.qq.com:587`（STARTTLS），`.env.example` 已对齐：

```sh
docker compose --env-file ../web-arrebyte/.env --env-file .env up -d --build
```

服务器已存在 `web-arrebyte_default` 网络，Garage 在其中有 `garage` 服务别名。新版 Compose 会把菜谱容器接入该网络，通过 `http://garage:3900` 访问对象存储。若博客项目名改变，在 `.env` 中调整 `BLOG_DOCKER_NETWORK`。当前线上菜谱容器仍只连接旧的 `arre-cook_default` 网络，因此必须用新版 Compose 重建容器；单纯重启旧容器不会启用 S3。

## Garage 上传密钥

博客 Garage 已有 `blog-uploads` bucket 和博客专用密钥。菜谱应用应在同一 Garage 创建独立的 `cook-uploads` bucket 与专用密钥，只给该 bucket 读写权限，避免菜谱服务取得博客图片的访问权。当前 Garage 尚无菜谱专用密钥：

```sh
cd /home/ubuntu/web-arrebyte
docker compose --profile garage exec garage /garage bucket create cook-uploads
docker compose --profile garage exec garage /garage key create arre-cook
docker compose --profile garage exec garage /garage bucket allow --read --write cook-uploads --key arre-cook
```

把创建密钥时输出的 access key 和 secret 写进菜谱应用服务器上的 `.env`：`COOK_S3_ACCESS_KEY`、`COOK_S3_SECRET_KEY`，不要使用博客站已有的 S3 凭据。上传对象放在 `cook/u/<user-id>/...` 前缀；服务端会校验删除权限。菜谱应用自身的 `/uploads/` 路由负责读取历史本地图片和 Garage 图片，无需改博客站的公开图片路由。

首次启动新版前先用 SQLite 在线备份旧库。线上旧库位于 `arre-cook_ninimenu-data` 卷，当前使用 WAL，不能只复制 `ninimenu.db` 而漏掉 `-wal` 文件。备份后先在隔离环境迁移副本并核对数据，再升级线上服务：

```sh
mkdir -p /home/ubuntu/backups/arre-cook
backup_path="/home/ubuntu/backups/arre-cook/pre-multiuser-$(date +%Y%m%d-%H%M%S).db"
sqlite3 /var/lib/docker/volumes/arre-cook_ninimenu-data/_data/ninimenu.db ".backup '$backup_path'"
sqlite3 "$backup_path" 'PRAGMA integrity_check;'
chmod 600 "$backup_path" /home/ubuntu/arre-cook/.env
```

完整性检查应输出 `ok`。启动时会自动把旧版个人数据归到 `ADMIN_EMAIL` 对应账号；第一次登录该邮箱后检查记录、收藏、偏好和上传图片，再开放新用户注册。

## 反向代理

将 `cook.arrebyte.top` 的 HTTPS 虚拟主机接到博客 Docker 网络内的 `ninimenu:8080`。API、H5 和 `/uploads/` 都由同一个 Go 服务处理。反向代理要传递原始 Host、客户端 IP 和 HTTPS scheme；生产环境的 `PUBLIC_URL`、`CORS_ORIGINS` 应与实际域名一致。

应用也绑定宿主机 `127.0.0.1:9925` 供本机检查：

```sh
curl -fsS http://127.0.0.1:9925/healthz
```

首次创建管理员之后，设置 `ALLOW_REGISTER=false` 可以关闭公开注册。邮箱验证码有服务端冷却和 IP 限流；SMTP 未配置时生产环境无法登录。

## AI 与智能体

在管理员设置中配置大模型连接，或在服务端提供 `LLM_BASE_URL`、`LLM_API_KEY` 和 `LLM_MODEL`。密钥只由服务端读取。未配置模型时，站内助手仍使用本地推荐引擎。

每位用户在「我的 → AI 连接」创建自己的访问令牌。外部 Agent 和 MCP 客户端必须使用该用户的令牌；不要配置旧的全站 `AGENT_TOKEN`。发给第三方的令牌可以撤销、限范围，并在调用记录中查看。

## 数据迁移检查

备份数据库之后再升级。升级启动后至少检查：

- `ADMIN_EMAIL` 登录后能看到原账户的记录、收藏和评分；旧全局周计划缓存会清除并按用户重新生成。
- 新注册的第二个账号看不到管理员的个人记录、私房菜、建议和对话。
- 以只读 Agent 令牌调用写入工具会被拒绝；撤销令牌后立即失效。
- 上传图片的地址可通过 `/uploads/` 读取，普通用户无法删除别人的对象。

本地自动化验证：`cd backend && go test ./... && go vet ./...`，以及 `cd frontend && npm run build && npm run lint`。生产数据库迁移仍应先在副本上演练。
