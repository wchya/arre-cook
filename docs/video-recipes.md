# 视频做法提炼

Web 和原生微信小程序的菜谱编辑页提供「从视频提炼」。粘贴 B 站 / 抖音直链、短链或包含一个链接的分享文案，点击后读取真实字幕并生成草稿。没有可读取字幕时，启用语音转写的站点会尝试读取公开视频的音轨；用户也可以粘贴字幕或视频文稿。支持随当前应用运行的本地 SenseVoice，不需要独立 ASR 服务。只按字幕 / 语音整理，不做画面 OCR 或视觉动作推断。

## 填入与保存

- 提炼菜名、食材、用量、调料、制作步骤及字幕明确说明的时间和提示。未知用量保持空白，未知时间为 0，不用视频时长代替烹饪时间。总时间须提供明确说明整道菜总用时的原句；服务端检查时间与原句是否一致，不将各步骤相加，不将单独炖煮时间当作总时间。没有依据或不能准确识别的时间归零。
- 食材和步骤必须带连续的字幕原句。生成模型只返回字幕起止行号，服务端校验范围并从原文取回完整引用，避免模型在引用中换行、漏词或拼接；随后校验用量有依据并审核完整结果。标题、简介、背景音乐和推荐视频不作为做法来源。
- 用量跨字幕行时仅合并空白字符，保留上下限等限定语和原始引用；例如桂皮的“一点点 / 不要超过一克”不会丢掉后半句。无原文依据的用量仍拒绝填入。
- SRT / VTT 字幕中单独成行的数字也会保留，例如“500 / 克”或“30 / 分钟”，避免把食材用量和步骤时长当成字幕序号丢弃。
- 只填入开始提炼时与返回时都为空、期间未被编辑过的字段。已有内容保留；用户可展开结果核对。改链接、取消或离开页面会停止请求，旧结果不能覆盖新输入。
- 编辑页将结构化食材和步骤随本地草稿保存，避免复杂用量或多行步骤在恢复、保存时被重新拆分。
- 提炼接口不新建或修改菜谱，不产生对话记录。只有用户点击编辑页的保存按钮才提交菜谱，原有个人 / 家庭权限规则继续适用。

视频画面里的文字与平台提供的字幕文件是两回事。公开视频也可能没有可下载的字幕文件，或仅向已登录用户提供；此时自动提炼需要配置语音转写，不能把“平台未返回字幕”理解为画面没有字幕。Web 预览和详情页提供经过校验的平台链接；小程序无法直接打开时明确提示复制链接到对应平台播放。保存时只规范化正在保存的菜谱的视频链接，历史数据不做批量改写。

## 服务配置

提炼使用现有服务端文本模型配置（CPA 或 `LLM_*`），不启用模型时明确提示手动填写。B 站先读取 `/x/v2/dm/view` 的目标分 P 字幕，正常响应但无可用字幕时才回退播放器字幕入口；优先人工中文，再选中文 AI 字幕。平台拒绝或限流时停止，不改用另一个入口绕过限制。

### 抖音游客凭据

抖音匿名分享页可能返回 HTTP 200 的验证页面，页面中没有视频数据。可以配置运维提供的游客凭据，通过工作台 / yt-dlp 已验证的 `/aweme/v1/web/aweme/detail/?aweme_id=…` 获取目标视频。服务器直接使用现有 Go 网络读取器，不安装浏览器、yt-dlp 或完整工作台。未设置 `VIDEO_DOUYIN_COOKIE_FILE` 时保留匿名页面读取；设置后若凭据缺失、格式无效或过期，会明确停止并提示更新。

在运维自己的电脑上运行以下命令（Python 3.10+，已安装 Chrome 或 Edge）：

```sh
python3 -m venv .venv-douyin
.venv-douyin/bin/python -m pip install -r scripts/requirements-douyin.txt
.venv-douyin/bin/python scripts/douyin-guest-cookies.py --output secrets/video/douyin-guest.json
```

Windows 将 Python 命令改为 `.venv-douyin\Scripts\python.exe`；需要时用 `--browser` 指定浏览器可执行文件。工具使用全新的独立浏览器目录，阻止媒体加载，不访问日常浏览器账号；取得游客凭据后关闭并清理浏览器目录。检测到登录态或交互验证码就停止，失败不覆盖原有凭据。不要在窗口登录，也不要使用账号 Cookie 导出文件替代本工具生成的 JSON。

将生成的 JSON 通过运维安全通道放到服务器项目目录的 `secrets/video/douyin-guest.json`。这只是小型凭据文件，不是视频文件。镜像中的应用 UID / GID 为 `1000:1000`，服务器上设置：

```sh
sudo chown 1000:1000 secrets/video secrets/video/douyin-guest.json
sudo chmod 700 secrets/video
sudo chmod 600 secrets/video/douyin-guest.json
```

在服务器 `.env` 设置 `VIDEO_DOUYIN_COOKIE_FILE=/run/video-secrets/douyin-guest.json`，按正常流程发布并重建容器。Compose 将整个 `secrets/video` 目录只读挂载到 `/run/video-secrets`；目录和凭据均不进入 Git 或构建上下文。后续更新时在同目录准备好权限和属主正确的新文件，再原子替换旧文件；读取器每次请求重新读取凭据，无需重启，相关缓存按凭据摘要隔离。不要直接截断正在使用的文件，也不要替换挂载目录本身。非 Compose 部署配置应用可读的绝对文件路径，仍要求文件权限 `600`。

凭据只发给固定的抖音详情接口，不发给短链、分享页、媒体 CDN、字幕、B 站或 ASR；接口跳转也停止。平台返回验证页、空详情响应或 403 / 412 / 429 时进入冷却，不自动重试、启动浏览器或轮换凭据。凭据失效由运维在本机重新生成；更新凭据不会清除平台冷却和累计请求预算。

获得元数据后仍优先字幕，无字幕时优先可识别的独立音轨，否则从允许的播放地址中选择已知体积最小的媒体源，并优先使用直接 CDN 地址。保留 10 分钟 / 20 MiB 上限；已知最小资源超限时在下载和 ASR 前拒绝，实际读取仍有字节限制。没有独立音轨的视频仍需临时读取含音轨的视频并做现有本地识别，因此这项修复不能消除所有媒体带宽和 ASR 开销，也不新增用户下载后上传的入口。

### 本地语音转写

部署镜像包含 FFmpeg、Python 3.12、`sherpa-onnx` 与 NumPy，不包含工作台或另一个监听端口。使用固定版本的 SenseVoiceSmall int8 和 Silero VAD；模型文件合计约 229 MiB，不在每次请求时下载。

先在仓库根目录安装并校验模型：

```sh
python3 backend/video_asr/download_models.py --destination models/video-asr
```

然后在服务端 `.env` 设置 `VIDEO_ASR_PROVIDER=local`，使用现有 Compose 构建并重建食谱容器。模型只读挂载到 `/app/models/video-asr`，任务音频写入 `/app/video-work` 的 64 MiB tmpfs，完成、失败和取消后清理。网络不通时可在可联网的机器下载后传到服务器，仍须通过清单的大小和 SHA-256 校验。

本地路径不需要 ASR API Key，不会在失败后自动转用收费 API。提炼食谱仍调用现有文本模型；本地文字提取也会消耗服务器 CPU、内存和带宽。代码、模型来源和许可见 [第三方说明](../backend/video_asr/licenses/NOTICE.md)。

本地模式要求 Linux amd64 / arm64、Landlock 和 libseccomp。首次状态检查会验证模型哈希、原生依赖和沙箱能力；缺失时 `asr_enabled=false`，保留字幕与手动文稿路径。非 Docker 部署必须提供相同的受控运行路径；macOS / Windows 构建仍可使用字幕和远端转写。

### 可选远端转写

`VIDEO_ASR_PROVIDER` 默认为 `remote`，保持旧部署兼容。此模式使用独立配置，不假定文本模型支持音频，也不复用 CPA 凭据：

| 环境变量 | 含义 |
| --- | --- |
| `VIDEO_ASR_PROVIDER` | `local` 使用内置 CPU 模型；`remote` 使用下面的服务配置 |
| `VIDEO_ASR_URL` | 完整 HTTPS 转写地址，如 `https://api.openai.com/v1/audio/transcriptions` |
| `VIDEO_ASR_API_KEY` | 转写服务的独立密钥，仅服务端持有 |
| `VIDEO_ASR_MODEL` | 该服务支持的转写模型，例如 `gpt-4o-mini-transcribe` |

远端模式下，URL、API Key 和模型三个值全部有效才启用语音回退。请求是 OpenAI 兼容的 multipart：`file`（带扩展名及 MIME）、`model`、`response_format=json`；响应读取 `text`。支持识别 MP4/M4A、MP3、WAV、FLAC、Ogg、WebM，其他格式停止并提示粘贴字幕。不调用 shell 下载器，不让用户指定服务地址或模型。远端 ASR 不跟随重定向；音视频文件只临时处理，不作为菜谱附件持久保存。

协议参考：[OpenAI 官方 SDK 的转写接口](https://github.com/openai/openai-python/blob/main/src/openai/resources/audio/transcriptions.py)。实际可用模型、费用及数据处理规则以所配置服务商为准。

## 请求和安全边界

| 项目 | 限制 |
| --- | --- |
| 视频时长 / 媒体大小 | 10 分钟 / 20 MiB；无法确认时长时不调用 ASR |
| 字幕正文 | 20–8000 字，最多 32 KiB；请求体最多 40 KiB |
| 网页 / 元数据 / 字幕响应 | 2 MiB / 1 MiB / 512 KiB，超限拒绝，不截断后继续生成 |
| 网络 | 白名单平台域名和路径、HTTPS、最多 3 次跳转；每次连接拦截私网、环回、CGNAT、链路本地、测试网、组播、保留与 IPv6 转换地址 |
| 平台请求 | B 站、抖音各自全站间隔至少 2 秒，每小时最多 60 次、UTC 日最多 200 次；ASR 也有独立的同等预算 |
| 平台冷却 | HTTP 403 / 412 / 429、B 站风控错误、抖音验证页或空详情响应触发至少 15 分钟冷却；`Retry-After` 最多采纳 24 小时 |
| 抓取缓存 | 预览、保存元信息和提炼共用；最多 128 条 / 20 MiB，有效期 10 分钟，读取失败缓存 30 秒 |
| 字幕缓存 | 仅公开内容，最多 64 条 / 2 MiB，有效期 10 分钟；手动字幕及账号额度错误不进入共享缓存 |
| AI 额度 / 并发 | 与站内助手共用个人每日 20 次、全站默认 200 次和个人 1 / 全站 4 个请求位置；管理员原配置仍生效 |
| 本地 ASR 并发 / 内存 | 单应用进程同时 1 个任务，不排队；开始时至少 768 MiB 可用，子进程 RSS 超过 512 MiB 或服务器余量过低即停止 |
| 本地 ASR 解码 / 分段 | 仅解码本地文件，校验实际音轨不超过 600 秒；识别片段硬性不超过 6 秒，串行处理，ASR 2 线程 / VAD 1 线程 |
| 超时 | 整体 90 秒，本地 ASR 最长 50 秒、远端 ASR 最长 40 秒；模型输入 / 输出审核各最多 20 秒 |
| 审核推理预算 | 普通审核最多 512 token，逐项核对视频字幕与菜谱时最多 1536 token；只接受完整的 `ALLOW` / `BLOCK`，不自动重试 |
| 菜谱生成预算 | 最多 4096 token，模型原始正文最多 16 KiB；服务端还原引用后仍按字段和每段 4–240 字校验，不再套用模型正文的字节上限；截断结果不会填入或自动重试 |

GLM-5.3 / GLM-5.3-Flash 的结构化生成和审核显式使用 `reasoning_effort=low`。这些模型默认最高推理强度且不能关闭思考，可能在返回 JSON 前耗尽额度；其他模型不附加该参数。仍要求完整结果、逐项字幕依据和审核通过，整体 90 秒与审核 20 秒的期限保持不变。模型约束见 [智谱官方文档](https://docs.bigmodel.cn/cn/guide/models/text/glm-5.3)。

CPA 应通过智谱原生 Chat Completions 兼容入口转发这些请求；Anthropic 协议转换可能丢失 GLM 的推理强度，导致预算用完仍没有 JSON。可在 CPA 内配置独立前缀（如 `cook/glm-5.3`），并通过应用服务端的 `LLM_CPA_MODEL` 选择。该覆盖仅调整模型，仍使用挂载配置中的 CPA 地址和密钥；没有配置覆盖时沿用原模型。不得把上游订阅密钥下发给客户端。

平台间隔、日 / 小时预算和冷却存于 `video_platform_budgets`，跨重启、部署、实例生效；同进程相同资源的并发请求合并。数据库失效时停止抓取。公网抓取不用环境代理、登录 Cookie 或用户账号凭据；可选的抖音游客凭据仅用于上述固定详情接口。每次实际拨号检查解析后的 IP，防止 DNS 重绑定。登录 JWT、来源检查、认证前 IP 限流、每账号提炼 12 次/分钟、预览 30 次/分钟共同保护入口。PAT 和 Agent 会话不可调用。

本地子进程不继承数据库、JWT、SMTP、CPA 凭据。Landlock 仅开放当前任务目录、只读模型及运行库；seccomp 拦截联网、进程窥探以及旧内核未覆盖的截断和权限修改操作。FFmpeg 的格式、协议、解码时间、CPU、地址空间和输出大小均受限。取消或异常退出时回收整个进程组。Compose 保留默认 seccomp，移除 capabilities、禁止提权并限制进程数量。扩容前需增加跨实例计算任务准入，不能直接把本地 ASR 并发叠加到资源紧张的服务器。

只有实际获得 ASR 资源和请求预算、或即将进入模型处理时才一次性消耗 AI 额度；此后取消、失败或审核拒绝不返还。忙碌、资源不足、缺字幕、平台拒绝读取，以及在 AI 处理前发现的格式 / 时长 / 大小限制不消费 AI 次数。平台读取预算仍按实际网络尝试计数。

审核异常只记录阶段、失败类别、完成状态、token 上限及响应字节 / 工具数量，便于区分截断、格式错误和上游不可用；不记录字幕、模型正文、推理内容、工具参数或凭据。

平台策略会变化，公开接口也可能要求登录、验证或限制服务端访问。系统不破解签名、验证码，不轮换代理或 Cookie 绕过限制。字幕 / 转写及模型审核可能出错，保存前需核对，不能承诺绝对无漏洞或永不限流。

## 接口

- `GET /api/assistant/video-recipe/status`：返回 `enabled`、`asr_enabled`、输入限制和当前账号 `quota`。
- `POST /api/assistant/video-recipe`：只接受 `{ "url": "...", "transcript": "可选字幕正文" }`。
- SSE：`quota` → `status` / `ping` / 更新后的 `quota` → 审核后的 `recipe` → `done`；失败为 `error` → `done`。`recipe` 包含 `recipe` 和 `source`（`method` 为 `subtitle` / `audio` / `manual`）。
- 15 秒心跳与 `X-Accel-Buffering: no` 防止代理空闲断流。客户端收到完整结果及 `done`，且请求未取消后才应用；断流不自动重发 POST。

## 验证

后端回归位于 `internal/video/video_test.go`、`internal/services/video_budget_test.go`、`internal/assistant/video_recipe_test.go`、`internal/routes/video_recipe_test.go`；客户端回归位于 `scripts/tests/video-recipe.test.cjs` 和既有草稿、私房菜可见性测试。覆盖 SSRF、跳转、字节上限、持久预算与冷却、分 P、抖音目标匹配、缓存 / 取消、ASR multipart、审核 / 截断 / 注入、无隐式写入、双端 UTF-8 分块和草稿保护。

抖音补充回归位于 `internal/video/douyin_detail_test.go` 和 `scripts/tests/douyin-guest-cookies.test.py`，覆盖游客凭据作用域、替换后的缓存、最小媒体、验证响应、登录态拒绝和私密内容拦截。默认不访问抖音；可在 `backend` 目录设置 `ARRE_DOUYIN_TEST_COOKIE_FILE` 为本机凭据绝对路径，运行 `go test ./internal/video -run '^TestDouyinLiveMetadata$' -count=1 -v`，显式验证元数据，不下载视频、不调用 ASR。

2026-09-28 本机实测：用户短链 `https://v.douyin.com/Z163A0D1xSs/` 对应 `7673525775051948287` 的花椒烤鸡腿视频。新 Go 详情读取测试通过，接口时长向上取整为 52 秒，选出 4,550,389 字节（约 4.34 MiB）的媒体源。本次平台返回的源没有独立音频或字幕，验证未下载媒体、未执行该视频的 ASR，也未在生产部署或验证抖音完整提炼；不能把元数据成功视为完整文稿已提取。

上线前运行 Go 测试 / vet、Web lint / build、客户端测试，并检查移动端布局。Python 边界测试为 `python3 -m unittest discover -s backend/video_asr`；Linux 镜像内的 `test_sandbox.py` 验证配置读写、截断、联网及 shell 执行被拒绝。可在镜像内为编译后的 Go 测试设置 `ARRE_VIDEO_ASR_TEST_MEDIA`，运行 `TestLocalASRNativeIntegration`，覆盖真实转写、取消、静音、损坏媒体、实际超长音轨及临时文件清理。

`TestVideoRecipeLiveProvider` 为可选的真实模型验收：显式提供 `ARRE_VIDEO_RECIPE_TEST_TRANSCRIPT` 公共字幕文件及只读 CPA 配置后运行，覆盖真实视频字幕、原文行号引用与复杂时间。该测试会调用模型，默认跳过，不访问数据库、不抓取平台、不保存菜谱。

2026-09-28 已在生产服务器的受限测试容器中验证用户视频 `BV1Vo1zB2EZf` 的真实音轨和本地识别链路；该视频也可直接读取公开中文字幕，正常导入应优先走字幕。平台可用性及模型输出仍须在发布环境检查，微信组件模拟不替代真机验收。

2026-09-28 后续修复已在服务器隔离环境通过真实模型验收：该公开视频的 158 段中文字幕（1396 字）经行号引用还原，保留“三斤鸡块”和桂皮“不要超过一克”的限制。字幕直接提炼耗时约 14 秒；完整短链读取、审核及 SSE 接口耗时约 25 秒，返回 `recipe → done`，只消费一次测试账号额度，未写入菜谱或聊天记录。复杂时间样例也保留总用时为 0。

实际字幕及已审核结果已固化为 `internal/assistant/testdata/video_chicken_citations.json`，常规回归无需联网即可检查原文引用和用量限制。该批修复纳入 v0.12.4；实际发布状态以发布记录和 `/healthz` 为准。隔离测试不替代微信真机验收，也未覆盖所有平台视频。
