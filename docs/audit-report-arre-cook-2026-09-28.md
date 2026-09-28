# Fuck My Shit Mountain Audit Report

**Project:** arre-cook（arre 食谱推荐小助手）
**Audit mode:** full，重点为安全、性能、可扩展架构与极端场景
**Date:** 2026-09-28（Asia/Shanghai）
**Reviewer:** Codex
**审查基线:** 核心业务代码 0872efc / 0.12.4；收尾时本地与线上均为 81861e4 / 0.12.5。两者差异仅为小程序底栏样式和版本号，不影响本文业务结论。
**工作方式:** 全仓清点与风险检索、关键路径逐函数审阅、隔离数据库故障注入、客户端模块验证、线上配置只读核验。未修改业务代码，未对线上施压或投递恶意样本。

---

## 1. Executive Summary

**当前线上服务正常，但不宜把当前低负载下的稳定，等同于对异常请求、数据库故障和并发操作已有充分保护。** 共记录 **19 项问题：6 项 High、13 项 Medium**。其中 18 项确认了代码缺陷、暴露路径或配置缺口，1 项多实例问题仍需部署环境验证；“Confirmed”不表示线上已经发生事故。最优先的是 WebP 解码依赖漏洞、菜谱数量与菜单计算上限、安全状态更新以及普通 API 的资源准入。

线上是 **4 核、3.32 GiB 内存、无 swap、13 个容器共用一台主机**。内存可用量采样约 0.89–0.93 GiB。0.12.4 起已生效 Go 的 256 MiB 软内存目标、图片上传单并发与有界等待、周菜单移除进程常驻缓存；这些改动有实际价值。但 Go 软目标不是容器硬上限，也不覆盖 Python ASR。单个账号的大菜库、恶意 WebP、未限长普通 JSON 与本地 ASR叠加，仍可影响同机 MySQL、博客和其他服务。本文未测得线上 OOM，也未测得当前业务存在容量瓶颈。

权限隔离、JWT/PAT 校验、视频 SSRF 防护、ASR 沙箱、AI 配额和审核链条已有较扎实的基础。主要问题不是必须重写架构，而是**同一业务规则分散在多个入口、数据库操作没有统一的取消与错误契约、后台重计算缺少有界调度，以及客户端身份状态不是唯一事实来源**。建议先做下面的局部修复，再考虑拆分 ASR。

### Score Dashboard

分数为基于本次证据的工程判断，10 分最好，不代表安全认证；综合分为七项平均值。

~~~
Security        ██████░░░░  5.5  B  认证基础可靠，但解码漏洞、安全状态回写和身份错位有可达路径。
Stability       █████░░░░░  5.0  B  已复现故障返回成功、跨表半提交和业务并发不变量被破坏。
Performance     █████░░░░░  5.2  B  小主机上仍有全量读取、二次复杂度及普通 API 准入缺口。
Testing         ███████░░░  7.0  A  现有边界测试有价值，但缺少 MySQL 故障、业务交错和多标签页测试。
Maintainability ███████░░░  6.5  B  模块分工清楚，规则在 handler/service、迁移模型表之间已出现漂移。
Design          ██████░░░░  6.0  B  AI 配额有原子设计，普通写操作与派生状态尚未统一事务边界。
Release         ██████░░░░  6.0  B  有健康检查及非 root 镜像，但依赖门禁、排空和迁移完整性不足。
────────────────────────────────────────────────
Overall         ██████░░░░  5.9  B
~~~

### Finding Statistics

| Severity | Count | Confirmed | Suspected |
|---|---:|---:|---:|
| Critical | 0 | 0 | 0 |
| High | 6 | 6 | 0 |
| Medium | 13 | 12 | 1 |
| Low | 0 | 0 | 0 |
| Info | 0 | 0 | 0 |
| **Total** | **19** | **18** | **1** |

## 2. Project Map

| 组成 | 职责与状态 | 主要审查边界 |
|---|---|---|
| frontend | React、Zustand、TanStack Query；token 存 localStorage | 跨标签页身份、请求结果归属、草稿、流式结果 |
| miniprogram | 微信原生页面及 WebView 桥接 | token 传递、页面生命周期、旧请求、文本与图片 |
| backend/cmd/server | Gin HTTP、定时清理、优雅关闭 | 入站限流、超时、部署排空、后台任务 |
| handlers / services / models | 个人/家庭菜谱、记录、菜单、清单、成就、通知 | 权限、事务、不变量、批量上限、错误处理 |
| agent / assistant / llm | REST/MCP 工具、对话、视频食谱提炼 | scope、配额、工具执行、供应商响应 |
| video / video_asr | 平台元数据/字幕、音频下载、FFmpeg、SenseVoice | SSRF、沙箱、模型校验、内存与并发 |
| MySQL / Garage | 关系数据、图片对象 | 连接池、锁等待、存储额度、数据生命周期 |
| 构建与迁移工具 | Docker 多阶段构建、SQLite→MySQL 迁移 | 依赖版本、模型表完整性、运行时版本 |

数据主线为客户端 → Nginx → Gin 鉴权 → 业务服务 → MySQL/Garage；AI 路径再访问 CPA，视频无字幕时启动本地 Python 子进程。资源隔离的实际边界是同一宿主机，不能只按 Go 进程空闲 RSS 估容量。

### 线上配置基线

最新健康核验时间 **11:55:12 +08:00**；资源数据为本轮多次短采样，不是峰值监控。

| 项目 | 实际值 | 判断 |
|---|---|---|
| CPU / 系统 | 4 核；Linux 5.15；x86_64；cgroup v2 | CPU 解码、PBKDF2、ASR 会争抢同一组 CPU |
| 内存 | MemTotal 3,481,024 KiB；MemAvailable 929,752–972,728 KiB | 可用约 908–950 MiB，不能按标称 4 GB 预算 |
| swap | 0 | 突发内存压力没有交换区缓冲 |
| 根磁盘 | 69G，总使用约 38G，可用约 29G | 当前未满；数据库和对象存储共享容量风险 |
| 应用 | 0.12.5 / 81861e4；健康；RestartCount=0；OOMKilled=false | 本次没有观测到 OOM/重启事故 |
| Go | 线上二进制 Go 1.25.14；GOMEMLIMIT=256MiB；GOMAXPROCS 未配置 | 本地扫描工具使用 Go 1.26.5，不能混为线上版本 |
| 应用容器 | 无 memory/CPU 硬限制；PIDs=256；非 root、cap_drop ALL、no-new-privileges | 进程数上限不能代替内存与 CPU 限制 |
| 同机负载 | 13 个容器；博客约 600.5 MiB，MySQL 约 407.8 MiB，应用空闲约 20 MiB | 空闲快照不能证明组合负载峰值安全 |
| 其他容器限制 | 博客 1.172 GiB；MySQL 1 GiB；Redis 384 MiB；Nginx 128 MiB；多个服务无限制 | 各服务预算需要统一校核 |
| MySQL | 应用池 20/5；全局 max_connections=151；buffer pool=512 MiB；锁等待=50s；packet=64 MiB | 当前 Threads_connected=9、历史峰值 11，尚未连接耗尽 |
| 当前数据 | dishes 约 600；behavior_events 约 530；其余主要表多为几十至百行 | 行数来自表元数据估计；规模问题属于未来增长/滥用风险 |
| Nginx | 请求体 20m；代理读写超时 300s；未发现 cook 专用 limit_req/limit_conn | 普通 API 缺少比 20 MiB 更细的请求准入 |
| 网络 | 应用仅绑定 127.0.0.1:9925；线上经 HTTPS 域名访问 | 没有把应用端口直接暴露公网 |
| ASR | local；SenseVoice int8；单进程任务槽；单任务 RSS 512 MiB；启动余量 768 MiB；50s | 保护存在，但启动检测不是跨任务资源预留 |
| ASR 文件 | 模型只读挂载；三个文件 SHA 校验匹配；work tmpfs=64 MiB | 不存在本轮证据支持的“模型未装好”问题 |
| ASR 运行时 | Python 3.12.14、sherpa-onnx 1.13.8、numpy 2.5.3 | 未执行真实线上转写或性能压测 |
| 注册 / 邮件 | ALLOW_REGISTER=true；SMTP 已启用 | 普通账号即可触达本文多项资源路径 |
| 日志 | json-file，10m × 3 | 控制日志容量，不等于对内容脱敏 |

### 范围与验证边界

全仓清点覆盖约 **271 个 Go/Python/TS/TSX/JS/脚本文件、46,757 行**，包括测试与静态菜谱数据。重点完整审阅认证、权限、数据写入、AI/视频、上传、计划缓存、部署，以及客户端会话关键实现；UI、静态资源、菜谱数据结合全仓检索和已有测试检查。**不是对每行代码的形式化验证，也不能保证找出了全部缺陷。**

没有获取业务私密内容来做样本，没有展示服务器凭据。未执行真实邮件、模型供应商请求、MySQL 故障注入或生产压力测试；本地故障注入使用临时 SQLite。MySQL 的锁语义、多实例行为、Linux 原生 ASR 的峰值仍需独立预发验证。

## 3. Top Risks

| 优先级 | Finding | 等级 | 一句话影响 |
|---|---|---|---|
| 1 | F01 WebP 解码依赖漏洞 | High | 已登录用户上传特制图片可触发过量内存分配，线上无容器内存硬边界 |
| 2 | F06 安全状态写失败/版本回退 | High | “退出全部设备”可能未生效；延迟改密可重新激活已撤销的一代 token |
| 3 | F02 复制绕过菜谱数量配额 | High | 500 道上限不覆盖全部入口，可建立无界个人菜库 |
| 4 | F03 菜单参数与计算量无上界 | High | 大菜库与超大餐次数量组合导致二次计算和大对象序列化 |
| 5 | F04 普通 API 缺少准入 | High | 普通 JSON、密码计算和公共回源缺少资源级限流 |
| 6 | F05 数据库缺少取消与超时 | High | 客户端离开后查询仍执行，锁等待/网络异常可占满 20 个连接 |
| 7 | F15 同机 ASR 总预算不足 | Medium | 独立限流器和内存采样不能防住不同重任务同时增长 |
| 8 | F07/F08 客户端身份错位 | Medium | 标签页显示 A 却以 B 身份提交，或链接替换已有会话 |
| 9 | F09 多表业务半提交 | Medium | 记餐成功但清单缺失；建议写入失败仍被标记采纳 |
| 10 | F10 家庭转让与退出竞态 | Medium | 转让成功后家庭可以失去任何有效管理员 |
| 11 | F11/F12 缓存与成就重计算 | Medium | 修改菜单源不失效、单账号重计算拖住其他账号 |
| 12 | F16/F18 发布与迁移完整性 | Medium | 发布截断长任务；迁移漏复制已上线的家庭业务表 |

## 4. Detailed Findings

### F01：线上 WebP 解码依赖存在已公开的内存耗尽漏洞

- **Severity:** High；**Confidence:** High；**Category:** Security / Performance；**Status:** Confirmed（版本、入口、调用链；未投递恶意样本）。
- **Affected area / Evidence:** [backend/go.mod:11](../backend/go.mod#L11)，[handlers/upload.go:152](../backend/internal/handlers/upload.go#L152)，[imaging/compress.go:24](../backend/internal/imaging/compress.go#L24)。线上二进制确认 x/image=v0.41.0。允许 image/webp，随后执行 DecodeConfig 和 imaging.Decode。
- **Problem / Why it matters:** [GO-2026-6222](https://pkg.go.dev/vuln/GO-2026-6222) 指出 VP8L 可通过大量未使用的 Huffman tree groups 分配过量内存。依赖实现按 maxHGroupIndex 分配组，风险不等价于最终图片像素数。现有 5 MiB 文件、2400 万像素和单上传并发不能视为完整缓解。另有 [GO-2026-5061](https://pkg.go.dev/vuln/GO-2026-5061) 的 WebP 尺寸不一致 panic；Gin Recovery 可处理一般 handler panic，但不能恢复进程 OOM。
- **Realistic failure scenario:** 普通账号上传构造的 WebP，同时同机正在转写或数据库有内存增长；过量分配造成长 GC、进程或宿主机 OOM。当前没有证据表明已被利用。
- **Minimal fix:** 升级 x/image 至至少 v0.45.0，并验证所需 Go 版本；不能立即升级时暂时关闭 WebP 入站。镜像发布后核对二进制模块版本。
- **Better long-term fix:** 图片解码放入有内存硬边界的 worker；保留现有字节/像素/并发限制。
- **Regression test suggestion:** 在隔离且有限额的进程中执行官方 WebP 回归样本、尺寸不一致样本，验证内存上界和错误响应；重新扫描发布二进制。
- **Estimated effort:** 0.5–1 天；独立 worker 另计 2–4 天。

### F02：复制菜谱绕过 500 道数量配额，读路径又会加载整个菜库

- **Severity:** High；**Confidence:** High；**Category:** Security / Performance；**Status:** Confirmed，已复现。
- **Evidence:** [dish.go:236](../backend/internal/handlers/dish.go#L236) 的 CreateDish 检查 500 上限；[dish.go:326](../backend/internal/handlers/dish.go#L326) 的 CloneDish 直接 Create；[dish.go:120](../backend/internal/handlers/dish.go#L120) 随机列表先 Find 全部完整菜谱再截取。
- **Problem / Why it matters:** 配额只保护部分入口。同一用户可以反复复制可见菜谱，得到超过上限的完整记录。AI 配额与 AGENT_RATE_LIMIT 不保护普通复制路由。随机列表即使 pageSize 很小，也承担全菜库读取和内存成本。
- **Realistic failure scenario:** 种入 500 道本人菜谱后，普通创建返回 400，复制返回 200，数量达到 501；继续复制扩大数据库，再调用随机列表、菜单和导出。在当前低内存主机上可以同时放大 DB 与应用负载。
- **Minimal fix:** 把“检查并占用菜谱额度”收敛到公共服务，覆盖创建、复制及家庭共享入口；事务内锁定所属用户/家庭额度，避免 check-then-insert 竞态。
- **Better long-term fix:** 随机选取先使用轻量 ID/元数据候选集，只读取入选菜谱详情；为对象总量建立计费/容量式配额。
- **Regression test suggestion:** 499 道时并发创建/复制，最终最多 500；到上限后所有入口一致拒绝。大菜库随机查询不读取所有 longtext。
- **Estimated effort:** 1–2 天。

### F03：每餐菜数可任意增大，周菜单计算接近 O(N²)，缓存重复保存完整菜谱

- **Severity:** High；**Confidence:** High；**Category:** Performance / Backend API；**Status:** Confirmed，已复现。
- **Evidence:** [manage.go:267](../backend/internal/handlers/manage.go#L267) 直接保存字符串设置；[plan_service.go:47](../backend/internal/services/plan_service.go#L47)、[122](../backend/internal/services/plan_service.go#L122) 仅判断正整数；[174](../backend/internal/services/plan_service.go#L174) 每选一道再遍历池；[80](../backend/internal/services/plan_service.go#L80) 序列化完整周计划。
- **Problem / Why it matters:** 没有每餐上限，count 大于候选数时虽然最终会停止，但会遍历选出当日几乎全部候选，重复筛选、加权与分配接近二次复杂度。跨天重复的完整菜谱又进入 JSON 缓存。移除进程常驻缓存没有消除生成和读取时的峰值。
- **Realistic failure scenario:** 本地设置 lunch_dishes_per_day=10000 成功；59 道测试候选全部出现在首日午餐。若合法大菜谱约 0.5 MB，500 道被连续七天纳入，单份序列化量即可接近 1.75 GB 的数量级；这是上界推演，未实际分配或在线测试。
- **Minimal fix:** 在 API 和读取设置的领域函数两层校验每餐数量，例如 1–10；清理已有越界设置；为菜单响应总大小设置上界。
- **Better long-term fix:** 缓存菜谱 ID/版本与少量展示字段；抽样算法一次生成顺序，避免每次重扫；同用户生成做 singleflight。
- **Regression test suggestion:** 0、负数、10000、整数溢出均按契约处理；固定最大菜库进行基准，校验分配量/响应量，并验证旧设置不会绕过新校验。
- **Estimated effort:** 参数止血 2–4 小时；算法/缓存调整 1–2 天。

### F04：普通 API 的请求体、计算频率与公共回源没有统一资源准入

- **Severity:** High；**Confidence:** High；**Category:** Security / Performance；**Status:** Confirmed，请求体路径已复现。
- **Evidence:** [ai_guard.go:24](../backend/internal/middleware/ai_guard.go#L24) 只匹配 AI/auth 等路径；[routes.go:49](../backend/internal/routes/routes.go#L49) 普通 app 组仅统一 UserAuth；[auth.go:193](../backend/internal/handlers/auth.go#L193) 直接进行密码校验/哈希；[store.go:145](../backend/internal/storage/store.go#L145) 公共图片不存在于本地即请求 S3。
- **Problem / Why it matters:** 普通 settings、records、dish 等路由没有同等 MaxBytesReader；未知字段也先由 JSON 解码器读入。线上 Nginx 仅提供 20 MiB 总门槛。修改密码的 PBKDF2 是刻意昂贵的计算，却不受 auth 登录限流；不同随机图片路径能形成未认证回源负载。
- **Realistic failure scenario:** 本地 PUT /api/settings 携带 2 MiB 未知字段仍返回 200。多次接近代理上限的请求会占用 Go 内存；普通账号反复改密消耗 4 核 CPU；随机图片路径增加 Garage 请求。这些是不同成本，不能由一个 AI 限流器覆盖。
- **Minimal fix:** 所有 JSON 默认设置字节上限，较大菜谱接口单独批准额度；批量数组和字符串另设业务上限。对改密、导出、随机查询分别设置用户/全局并发；公共回源加网关限速和并发保护。
- **Better long-term fix:** 统一入站策略表，按内存、CPU、DB、外部 I/O 成本划分额度；限流拒绝发生在读取大 body 和高成本计算之前。
- **Regression test suggestion:** chunked/无 Content-Length 的超限 JSON 返回 413；未知字段不能绕过；并发改密有确定的 429/排队边界；随机 404 回源保持并发上限。
- **Estimated effort:** 1–2 天。

### F05：普通数据库访问及健康检查不继承取消，也缺少连接级 I/O 超时

- **Severity:** High；**Confidence:** High；**Category:** Stability / Performance；**Status:** Confirmed，取消请求行为已复现。
- **Evidence:** [middleware.go:34](../backend/internal/middleware/middleware.go#L34) 全局 DB 鉴权；[config.go:178](../backend/internal/config/config.go#L178) DSN 无 timeout/readTimeout/writeTimeout；[routes.go:239](../backend/internal/routes/routes.go#L239) healthz 使用 Ping；业务服务普遍使用 database.DB。
- **Problem / Why it matters:** HTTP ReadTimeout 不是业务执行超时；客户端 15s 超时或取消不会终止这些查询。线上应用最多 20 个连接，MySQL 锁等待 50s，网络失联还可能更久。健康检查自己也可以陷入等待，外部 wget 退出不会自动释放未关联 context 的 DB 操作。
- **Realistic failure scenario:** 请求进入前 context 已取消，本地 /api/me 仍完成数据库鉴权并返回 200。线上若 20 个查询被锁/网络拖住，其他用户连鉴权都会排队；重试继续增加等待者。
- **Minimal fix:** request context 传入 handler/service/repository；普通请求设总超时，SQL 用 WithContext；DSN 加连接、读写超时；healthz 使用短 PingContext。
- **Better long-term fix:** 在数据库边界统一错误、取消与观测；设置 DB 等待队列/并发预算，统计连接池 WaitCount、WaitDuration。
- **Regression test suggestion:** 隔离 MySQL 中制造锁等待和网络中断，取消后连接及时释放，健康检查在约定时限内返回；不使用生产库验证。
- **Estimated effort:** 2–4 天，按业务模块分批。

### F06：改密/退出全部设备忽略写失败，改密还可能回退令牌版本

- **Severity:** High；**Confidence:** High；**Category:** Security / Stability；**Status:** Confirmed，两种情况均已复现。
- **Evidence:** [auth.go:216](../backend/internal/handlers/auth.go#L216) 基于已读取用户的 TokenVersion++ 后固定值回写且忽略 Error；[auth.go:225](../backend/internal/handlers/auth.go#L225) 忽略退出全部设备的更新错误；[middleware.go:37](../backend/internal/middleware/middleware.go#L37) 按 token version 等值判定会话有效。
- **Problem / Why it matters:** 数据库拒绝更新时仍签发/返回成功；用户以为安全操作完成。并发情况下，旧请求把数据库更高的 token version 覆盖成旧值加一，使已撤销的一代 token 再次匹配。
- **Realistic failure scenario:** 注入 users 更新失败后，改密和 logout-all 都返回 200，原密码状态/版本没变化，原 token 仍有效。另一测试将交错的撤销提交到版本 3，延迟改密又写回 2；版本 2 token 从 401 变为 200。
- **Minimal fix:** 检查 Error/RowsAffected；改密在事务内重新校验当前密码/版本，采用 CAS 或行锁，原子推进版本；仅在提交成功后签发基于新状态的 token。
- **Better long-term fix:** 把改密、禁用、会话撤销集中到认证服务，避免 handler 直接更新安全字段。
- **Regression test suggestion:** 写失败必须返回非成功且不签发新 token；并发改密/撤销时版本严格单调，已撤销 token 永不恢复；为管理员禁用同样注入失败。
- **Estimated effort:** 0.5–1.5 天。

### F07：Web 多标签页共享 token，却不共享身份与请求归属

- **Severity:** Medium；**Confidence:** High；**Category:** Security / Frontend State；**Status:** Confirmed，客户端模块已复现。
- **Evidence:** [api/client.ts:38](../frontend/src/api/client.ts#L38) 每次请求读取共享 localStorage；[useAuthStore.ts:20](../frontend/src/store/useAuthStore.ts#L20) 用户保存在每页 Zustand；[App.tsx:59](../frontend/src/App.tsx#L59) 仅监听当前页 store；没有 storage/BroadcastChannel 会话同步。成功响应无 token 世代检查。
- **Problem / Why it matters:** B 页切换账号后，A 页仍展示旧用户/草稿，而新请求已使用 B 的凭据。当前页切换时清 Query 缓存做得正确，但覆盖不了这个边界。[Account.tsx:23](../frontend/src/pages/Account.tsx#L23) 的迟到改密响应还会无条件 setSession。
- **Realistic failure scenario:** JSDOM 加载真实模块并发送 storage 事件后，A 页 user.id=1，但保存请求的 Authorization 已是用户 2 的 token。旧用户的私房菜内容可能被写入新账号。
- **Minimal fix:** 监听跨页会话变化，暂停提交、清理缓存、重取当前用户；mutation 绑定开始时的身份世代；成功结果也检查身份是否仍一致。
- **Better long-term fix:** 统一 session epoch，并纳入缓存 key、草稿 key、请求、回调与跨页广播。
- **Regression test suggestion:** 两页 A→B 切换、退出后旧成功响应、改密回包迟到均不能错写或恢复旧会话；复用小程序已有成功响应 token 校验思路。
- **Estimated effort:** 1–2 天。

### F08：普通网页接受任意 URL fragment 中的 token，存在登录会话注入

- **Severity:** Medium；**Confidence:** High；**Category:** Security；**Status:** Confirmed，客户端模块已复现。
- **Evidence:** [miniprogram.ts:57](../frontend/src/lib/miniprogram.ts#L57) 不要求 from=mp 或一次性 state 就返回 token；[useAuthStore.ts:27](../frontend/src/store/useAuthStore.ts#L27) 用它覆盖现有 token。
- **Problem / Why it matters:** 这是登录 CSRF/会话替换，不是 JWT 伪造。攻击者可提供自己合法账号的 token，引导用户在这个账号下录入数据。fragment 不进服务器日志解决了传输暴露的一部分问题，没有验证谁发起了登录。
- **Realistic failure scenario:** 普通浏览器已登录 A，打开本站带攻击者 B token 的链接，bootstrap 后变成 B；本地模块测试已验证，不需要小程序环境。
- **Minimal fix:** 普通网页禁止直接接收 bearer fragment；WebView 改用后端一次性交换码、短 TTL、消费一次，并绑定当前发起的登录流程/state。仅检查 UA 或 from 参数不足以构成安全绑定。
- **Better long-term fix:** 合并 Web/小程序登录交换协议，跨端传递短期授权码而不是长期会话。
- **Regression test suggestion:** 非发起流程链接不能替换会话；码重放、过期、state 不匹配均失败。
- **Estimated effort:** 1–2 天。

### F09：用餐记录、建议处理与买菜清单没有完整事务边界

- **Severity:** Medium；**Confidence:** High；**Category:** Stability / Data Integrity；**Status:** Confirmed，两个故障路径已复现。
- **Evidence:** [record_service.go:81](../backend/internal/services/record_service.go#L81) 先提交记录；[145](../backend/internal/services/record_service.go#L145) 清单 Create 忽略 Error；[agent_service.go:146](../backend/internal/services/agent_service.go#L146) ResolveSuggestion 忽略逐条记餐失败，仍 accepted；[family_service.go:440](../backend/internal/services/family_service.go#L440)、[shopping_service.go:209](../backend/internal/services/shopping_service.go#L209) 家庭计划与清单分开提交。
- **Problem / Why it matters:** 一次用户操作包含多个事实，但返回值只代表前半段成功，派生数据也可能和源数据不同步。家庭同格并发换菜还能产生菜单与清单不同版本。
- **Realistic failure scenario:** 注入 shopping_checks 创建失败，CreateMealRecord 返回 nil error、记录存在、清单为空。注入 meal_records 创建失败，建议仍变 accepted，返回空记录和 nil error。
- **Minimal fix:** 在同一事务中写源记录和同步必需的清单，通知放在提交后；建议处理用 pending 条件更新并处理过期状态，失败不消耗采纳状态；定义重复采纳的幂等返回。
- **Better long-term fix:** 对可延后副作用使用 outbox/重建任务，保留来源版本，避免跨事务假装全成功。
- **Regression test suggestion:** 各写入点失败均保持业务一致；并发换同一家庭菜单格后清单只属于最终菜谱；建议重复采纳不重复记餐。
- **Estimated effort:** 2–3 天。

### F10：家庭转让与成员退出之间存在 TOCTOU，家庭可失去有效管理员

- **Severity:** Medium；**Confidence:** High；**Category:** Stability / Authorization；**Status:** Confirmed，确定性交错已复现。
- **Evidence:** [family_service.go:296](../backend/internal/services/family_service.go#L296) 先检查目标成员，再单独更新 owner；[333](../backend/internal/services/family_service.go#L333) 退出先检查旧 owner，再删除成员。
- **Problem / Why it matters:** “owner 必须属于该家庭”的跨表不变量没有被同一锁或事务维护。
- **Realistic failure scenario:** 转让已通过成员校验，目标成员此时退出，再继续转让 UPDATE；两操作均成功。实测旧 owner 无权限，新 owner 因无 membership 也无法管理。
- **Minimal fix:** 转让、退出、踢人、解散使用一致的家庭行锁与事务次序，事务内再次判断成员和 owner；不只给单个方法加事务。
- **Better long-term fix:** 把家庭成员变更聚合为少数维护不变量的领域命令；同样保护 12 人上限。
- **Regression test suggestion:** 精确控制 transfer/leave、transfer/remove、join/delete 的交错；提交后 owner 必须是成员，成员数不超过上限。
- **Estimated effort:** 1–2 天。

### F11：普通菜谱修改/停用不使周菜单快照失效

- **Severity:** Medium；**Confidence:** High；**Category:** Correctness / Caching；**Status:** Confirmed，已复现。
- **Evidence:** [dish.go:278](../backend/internal/handlers/dish.go#L278)、[314](../backend/internal/handlers/dish.go#L314) 更新/停用未失效周缓存；[plan_service.go:60](../backend/internal/services/plan_service.go#L60) 仅按日期验证快照，并刷新收藏。
- **Problem / Why it matters:** 菜名、食材、是否启用变了，用户仍可读到旧完整菜谱直到重新生成或过周。改动过敏原相关食材时，旧菜单内容尤其容易误导。
- **Realistic failure scenario:** 预存本周菜单，HTTP PUT 修改菜名/食材返回 200，再读 GetCachedWeekPlan 仍得到旧值。公共/家庭共享菜谱需要考虑多个消费者，不能只失效操作者。
- **Minimal fix:** 统一所有菜谱变更入口的缓存失效；读取缓存时校验菜谱仍可见、启用且版本匹配。
- **Better long-term fix:** 缓存 ID 和版本，读取时批量补当前详情；明确历史用餐快照与“当前推荐菜单”不同的语义。
- **Regression test suggestion:** 修改、停用、删除和成员关系变化之后，个人/家庭/公共菜谱的缓存都遵守当前可见性与版本。
- **Estimated effort:** 0.5–1.5 天。

### F12：成就计算使用全局互斥锁并全量读历史，等待者没有容量边界

- **Severity:** Medium；**Confidence:** High；**Category:** Performance / Scalability；**Status:** Confirmed（结构与可达路径，未压测队列极限）。
- **Evidence:** [manage.go:66](../backend/internal/handlers/manage.go#L66) GET 同步触发计算；[achievement_service.go:179](../backend/internal/services/achievement_service.go#L179) 全局锁覆盖 DB 与计算；[250](../backend/internal/services/achievement_service.go#L250) 构建完整历史快照；[134](../backend/internal/services/achievement_service.go#L134) timer 删除队列标记后才进入计算。
- **Problem / Why it matters:** 一个大账号或慢 DB 会挡住全部用户的成就同步。300ms 防抖只合并尚未触发的 timer，不能限制正在等全局锁的请求/任务数；请求取消也不取消锁等待。
- **Realistic failure scenario:** F02 产生大菜库或用户积累大量记录，多页同时读成就，其他用户在全局锁后排队；持续写操作还会再产生等待的同步任务。
- **Minimal fix:** 每用户去重、全局有界 worker 队列；GET 读取最近已计算结果；锁只用于调度状态，不包住 DB I/O。
- **Better long-term fix:** 增量统计和版本化投影；跨实例以数据库唯一键保证成就幂等。
- **Regression test suggestion:** 慢账号任务存在时普通账号仍能读取；同用户千次触发只保留一个待处理标记；队列满有确定处理策略。
- **Estimated effort:** 1–2 天。

### F13：SMTP 只有建连超时，没有协议会话总超时/取消

- **Severity:** Medium；**Confidence:** High；**Category:** Stability；**Status:** Confirmed（代码与线上启用状态，未实际发送邮件）。
- **Evidence:** [mailer.go:34](../backend/internal/mailer/mailer.go#L34)、[44](../backend/internal/mailer/mailer.go#L44) 仅 Dial 10s；后续 NewClient/Auth/Mail/Data/Quit 无 SetDeadline；[auth_service.go:84](../backend/internal/services/auth_service.go#L84) 同步发送验证码。
- **Problem / Why it matters:** TCP 连接成功不代表 SMTP greeting 或 DATA 响应及时；外部服务半开会让 goroutine/socket 长期停留。每邮箱/IP 冷却的 count 与 create 也非原子，突发并发可超额。
- **Realistic failure scenario:** 邮件服务器接受连接但不发 greeting，HTTP 客户端早已超时，服务端仍阻塞。线上 SMTP 已配置，因此这不是未启用功能的风险。
- **Minimal fix:** Send 接受 context；建连后设全会话 deadline，取消时关闭 conn；为发送设置全局并发和原子预约额度。
- **Better long-term fix:** 有界邮件任务队列，持久化状态与重试，保证失败不无限重复投递。
- **Regression test suggestion:** 本地假 SMTP 分别停在 greeting、STARTTLS、DATA、QUIT，均在期限内返回且无 goroutine 泄漏。
- **Estimated effort:** 0.5–1 天。

### F14：上传只限制单文件，缺少累计配额及可靠的对象生命周期

- **Severity:** Medium；**Confidence:** High；**Category:** Security / Storage；**Status:** Confirmed（应用机制缺失，未观察到磁盘耗尽）。
- **Evidence:** [upload.go:185](../backend/internal/handlers/upload.go#L185) 保存新随机对象，无额度账本/引用记录；[auth.go:266](../backend/internal/handlers/auth.go#L266) 注销清理数据库但不触达对象存储；[store.go:145](../backend/internal/storage/store.go#L145) 对象可继续按 URL 读取。
- **Problem / Why it matters:** 上传后取消编辑、不发删除请求或注销账号，都可能留下对象。串行上传控制同时占用，无法限制累积占用。已知 URL 的残留也与“账号和个人数据永久删除”的产品表述不一致。
- **Realistic failure scenario:** 一个账号持续上传未引用图片，或日常取消编辑逐渐积累；Garage 与 MySQL 共享当前约 29G 可用盘。没有证据证明 Garage 自身设置了 bucket 配额，本文也不假设其一定无限制。
- **Minimal fix:** 用户/站点累计字节与对象数配额；上传先产生临时对象记录，保存业务时绑定引用，过期未引用对象回收。
- **Better long-term fix:** 按引用和保留策略做可重试的删除任务；家庭复制/共享图片存在多个引用，不能简单删除用户目录导致他人菜谱坏图。
- **Regression test suggestion:** 连续未引用上传达到配额即拒绝；取消编辑/注销后按契约清理；共享引用未释放时不误删。
- **Estimated effort:** 2–4 天。

### F15：本地 ASR 与 Go/图片任务之间没有共同内存预算，容器无限额

- **Severity:** Medium；**Confidence:** High；**Category:** Performance / Deployment；**Status:** Confirmed（隔离缺口），OOM 后果未实测。
- **Evidence:** [local_asr.go:44](../backend/internal/video/local_asr.go#L44)、[169](../backend/internal/video/local_asr.go#L169)、[224](../backend/internal/video/local_asr.go#L224) 独立槽与周期采样；[upload.go:41](../backend/internal/handlers/upload.go#L41) 独立上传槽；[docker-compose.yml:24](../docker-compose.yml#L24) 仅 Go 软目标。线上 inspect Memory=0、NanoCpus=0。
- **Problem / Why it matters:** ASR 的 768 MiB 启动检测不是资源预留。刚通过检测后，其他容器、图片处理、Go JSON 分配仍可增长。Python RSS watchdog 不等价于整个容器内存计量；多个副本各有一个 ASR 槽也不是宿主机共用一个槽。
- **Realistic failure scenario:** 启动时可用约 913 MiB，ASR 被允许；博客接近其 1.172 GiB 限制或同时处理大图片，余量很快缩小，先发生 GC/转写被杀，极端时触发系统 OOM。
- **Minimal fix:** 在 API 应用中统一重任务准入与预留；为所有同机服务核定峰值后设置容器限制；保留低内存拒绝。**不要直接把整个应用设成 256 MiB，也不要只加硬限制而不检查 ASR 的 768 MiB 启动条件。**
- **Better long-term fix:** ASR 独立 worker/容器，使用共享队列和硬限额；资源不足时降级为字幕/手动文稿。是否扩容需基于组合负载峰值，当前不能给出可信 QPS 上限。
- **Regression test suggestion:** 同配置预发同时运行最长视频、最大合法图片、菜单请求和博客/MySQL 负载，记录 cgroup memory.current/peak、PSI、拒绝率及延迟；保证拒绝而不是拖垮宿主机。
- **Estimated effort:** 配额核定与准入 1–2 天；拆 worker 3–5 天。

### F16：长请求的执行期限与发布排空期限不一致

- **Severity:** Medium；**Confidence:** High；**Category:** Release / Stability；**Status:** Confirmed（配置关系，未中断真实线上任务）。
- **Evidence:** [main.go:70](../backend/cmd/server/main.go#L70) Shutdown 15s；[assistant.go:71](../backend/internal/handlers/assistant.go#L71)、[video_recipe.go:64](../backend/internal/handlers/video_recipe.go#L64) 请求最长 90s；本地 ASR 最长 50s。Compose 没有 stop_grace_period；线上 StopTimeout 未设置。
- **Problem / Why it matters:** Docker 常规停止默认等待约 10s，比应用排空还短。更新镜像时进行中的 SSE/转写会被截断；即使外部部署命令另行延长 stop timeout，应用自身仍只等 15s。
- **Realistic failure scenario:** 用户已经消耗 AI 次数并开始任务，发布替换容器，执行和响应中断，前端只能重试或自行确认已完成的写操作。
- **Minimal fix:** 明确“允许完成”或“可恢复取消”的发布契约；统一代理切流、应用排空、Docker stop timeout；应先移出接流再等既有请求。
- **Better long-term fix:** 重任务使用持久 job ID 和状态，重连可查询，幂等键防止重试重复副作用。
- **Regression test suggestion:** 预发中任务执行一半发送 SIGTERM，验证完成/取消状态、配额语义、worker 清理与客户端恢复。
- **Estimated effort:** 0.5–1 天；持久任务化另计。

### F17：多实例下的初始化、限流与成就幂等尚未闭环

- **Severity:** Medium；**Confidence:** Medium；**Category:** Scalability / Release；**Status:** Suspected（静态交错有风险，未跑双实例 MySQL 验证）。
- **Evidence:** [main.go:31](../backend/cmd/server/main.go#L31) 每进程启动执行迁移/初始化；[notification_service.go:184](../backend/internal/services/notification_service.go#L184) 先读版本标记、批量发信再写标记，无显式争用行锁；[models/other.go:35](../backend/internal/models/other.go#L35) 成就用户组合是普通 index；[middleware.go:255](../backend/internal/middleware/middleware.go#L255) 限流器是进程内状态。
- **Problem / Why it matters:** 扩为多副本后，已有旧版本标记可被两进程同时读到而重复发升级通知；成就去重锁不跨进程；请求速率额度随副本数放大。初始化还一次读取所有用户并一次性批量插入通知，用户增多时增大启动事务。
- **Realistic failure scenario:** 两副本同时升级、账户请求轮询落到不同节点；代码事务并未自动提供业务幂等。目前单副本没有证据说明线上已发生此问题。
- **Minimal fix:** 独立运行版本迁移；版本通知使用可争用的唯一任务键/原子 claim；成就增加唯一约束；需要跨副本的速率额度迁至共享存储。
- **Better long-term fix:** 显式划分共享一致性状态和允许本地近似的缓存/限流；通知分批异步生成。
- **Regression test suggestion:** 两实例同连隔离 MySQL，重复启动、同时授奖、并发发通知，验证唯一性与总限流额度。
- **Estimated effort:** 2–4 天；单副本阶段可排在紧急修复之后。

### F18：SQLite→MySQL 迁移工具的模型清单已经漏表

- **Severity:** Medium；**Confidence:** High；**Category:** Release / Data Integrity；**Status:** Confirmed，模型清单对照。
- **Evidence:** [dbmigrate/main.go:45](../backend/cmd/dbmigrate/main.go#L45) modelsToCopy 缺 FamilyShoppingCheck、DishDeleteRequest；[database.go:75](../backend/internal/database/database.go#L75) 在线模型已包含两表；[dbmigrate/main.go:89](../backend/cmd/dbmigrate/main.go#L89) 实际先全表加载，再按 batch 插入。
- **Problem / Why it matters:** 工具成功退出不能表示完整迁移；家庭自动买菜记录/勾选和菜谱删除审批会丢失。batch-size 只控制写入批次，没有控制源表读取内存。
- **Realistic failure scenario:** 从含当前版本家庭功能数据的 SQLite 迁到新的 MySQL，工具显示完成但两表为空；大源表还可让迁移进程内存远超批次大小。目前线上已是 MySQL，不是正在发生的在线查询问题。
- **Minimal fix:** 补齐模型并建立迁移清单与 schema 注册表一致性检查；用 FindInBatches/游标读取；迁移后核对表、行数、关键关联。
- **Better long-term fix:** 版本化迁移与恢复演练，维护唯一模型清单，明确中断续跑/回滚语义。
- **Regression test suggestion:** 每种模型植入哨兵行，完整迁移后逐表验证，必须包含两张遗漏表和软删除记录；以大表验证峰值内存。
- **Estimated effort:** 0.5–1 天。

### F19：生产 GORM 错误日志默认展开 SQL 参数，可能记录密钥及个人内容

- **Severity:** Medium；**Confidence:** High；**Category:** Security / Observability；**Status:** Confirmed（日志配置与敏感写路径），未确认线上已有泄漏。
- **Evidence:** [database.go:33](../backend/internal/database/database.go#L33) 使用 logger.Default.LogMode(Error)，没有 ParameterizedQueries；[manage.go:326](../backend/internal/handlers/manage.go#L326) 管理接口可写密钥设置；[database.go:397](../backend/internal/database/database.go#L397) 值直接作为 SQL 参数。
- **Problem / Why it matters:** 生产只记 Error 并不等于脱敏。字段超长、约束冲突、数据库写入失败等场景会将展开的 SQL 送到日志，可能包含密钥设置、用户文本或密码哈希。CPA 挂载模式降低当前 llm_api_key 写入频率，但其他用户写路径仍存在。
- **Realistic failure scenario:** 管理员保存密钥或用户保存私人内容时数据库报错，日志读取者能够看到不应出现的参数；本次未扫描真实日志中的秘密来验证，也未声称外部攻击者可以直接读日志。
- **Minimal fix:** 生产启用 ParameterizedQueries，并对认证/设置路径使用字段级脱敏日志；记录 SQL 模板、错误码与关联 ID。
- **Better long-term fix:** 集中日志数据分类、访问权限与保留策略；审计内容和调试 SQL 分离。
- **Regression test suggestion:** 使用虚构敏感标记触发真实数据库约束错误，断言日志不包含标记，仍能定位语句和错误。
- **Estimated effort:** 2–4 小时。

## 5. Security Concerns

| 类别 | 结论 |
|---|---|
| 身份验证 | JWT 固定 HS256/issuer、要求 exp，数据库核验禁用与 token version；主要缺陷在 F06 写入一致性 |
| 授权隔离 | 个人/家庭作用域与 PAT scope 已检查；没有确认可直接枚举读取他人私房菜的漏洞 |
| 注入/XSS | 查询值参数绑定、排序白名单；未发现直接执行用户 JS/拼接 SQL 的实锤路径。不是对所有输入组合的证明 |
| SSRF / ASR | 协议/平台白名单、连接时公网 IP 检查、重定向与响应上界；ASR 有模型 hash、Landlock/seccomp、子进程权限隔离 |
| 资源滥用 | F01–F04、F14、F15 是当前优先项；“普通账号已鉴权”不能代替资源预算 |
| 浏览器身份 | F07/F08；同页 bootstrap/refresh 已有旧 token 防护，但多页和 mutation 未覆盖 |
| 敏感日志 | F19；此次未复制或输出实际凭据 |

没有把宽泛 TrustedProxies 直接判为可伪造来源 IP：线上 Nginx 追加真实 remote_addr，Gin 从右侧解析信任链，简单伪造 X-Forwarded-For 并不足以证明绕过。也没有把随机不可猜上传 URL 的公开读取直接判为越权；本文指出的是生命周期和回源成本。

## 6. Stability Concerns

高风险共同点是“数据库失败被当作业务成功”：安全操作 F06、清单/建议 F09、缓存保存和部分展示查询也存在忽略 Error。应先修能改变事实或权限的写路径，再统一只读接口如何报告不可用。

| 故障 | 当前表现 | 目标行为 |
|---|---|---|
| DB 写失败 | 部分 200、状态半提交 | 明确失败，不签发虚假新会话；跨表事务回滚 |
| 客户端取消 | 普通 SQL 继续 | context 到达 DB；释放连接/排队 |
| SMTP 半开 | 可无限等协议响应 | 总 deadline 和全局有界并发 |
| 同格/同账号并发写 | 旧值覆盖、不变量破坏 | CAS、行锁、唯一键和幂等 |
| 发布中断长任务 | SSE/转写截断 | 排空一致或任务可恢复 |

## 7. Performance Concerns

### 结合当前服务器的容量判断

**不能从约 20 MiB 空闲应用 RSS 推出还可以随意增加并发。** Go、Python、图片解码、序列化、MySQL buffer 和其他容器峰值需要按同一时刻相加。Go 256 MiB 是目标值，活跃对象过大时仍可能超过；内存 watchdog 是采样式保护，不能当作预先占用的容量。

| 场景 | 主要放大点 | 先采取的措施 |
|---|---|---|
| 单个大菜库 | 全量随机查询、推荐画像、导出 | F02 配额 + 轻量候选/分页 |
| 超大餐次数量 | 周菜单近 O(N²)、七天完整快照 | F03 双层范围校验与缓存瘦身 |
| 普通 API 突发 | body 分配、密码 CPU、DB 鉴权、S3 回源 | F04 分成本准入 |
| 依赖慢/数据库锁 | 20 个连接耗尽、等待 goroutine 累积 | F05 取消、超时、并发预算 |
| 长期活跃用户 | 成就/画像扫描完整历史 | F12 增量或有界重算 |
| ASR + 图片 + 博客峰值 | 同机内存余量失守 | F15 共同预算或拆 worker |

当前适合维持 **ASR 单任务**，优先修额度和准入，补 memory.current/peak、memory.events、PSI、Go heap/GC、DB 连接等待、任务耗时/拒绝率的观测。无需为尚未测量的负载先引入复杂集群；若希望频繁多人转写，应优先将 ASR 移出这台混部小主机，或在组合压测后扩容。

## 8. Testing Gaps

| 已完成验证 | 结果 | 实际提供的信心 |
|---|---|---|
| go test -race -count=1 -json ./... | 225 个测试及子测试 pass，2 skip | 当前覆盖下未发现 Go 内存竞态；不代表 DB 业务交错安全 |
| 前端/小程序 npm test | 36 pass | 草稿、流、生命周期和部分会话保护有效 |
| npm run lint / npm run build | 均成功 | 静态规则、TS 编译与打包通过 |
| go vet ./... | 成功 | 未发现该工具能识别的静态问题 |
| Python unittest | 6 个测试，5 pass、1 skip | 本地纯逻辑边界有效；Linux 沙箱未在 macOS 实测 |
| 本次隔离 Go 故障/交错验证 | 9 个复现用例全部得到预期坏行为，带 -race | F02/F03/F04/F05/F06/F09/F10/F11 的针对性证据 |
| 本次 Web 模块验证 | 2 个复现通过 | F07/F08 的身份错位与注入证据 |
| npm audit（官方 registry） | 0 advisory | 仅代表当时该审计源；镜像源审计接口 404 后改用官方源 |
| govulncheck（全包及 server） | 检出 12 条符号级告警 | 需按运行时、协议、格式和架构过滤，见第 17 节 |

明确跳过的 Go 用例是 TestLocalASRNativeIntegration、TestVideoRecipeLiveProvider。未测真实模型输出正确率、最长音频在本机的延迟/峰值，也未测真实 MySQL 锁竞争和双副本。这些缺口不能由单元测试全绿抵消。

[可复现证据与运行说明](audit-evidence/2026-09-28/README.md)。**复现测试断言的是当前缺陷存在，pass 不是修好了；修复时应反转为业务正确性断言。**

## 9. Maintainability Concerns

| 具体结构 | 已造成的风险 | 建议 |
|---|---|---|
| handler 和 service 都能创建/更新菜谱 | F02 配额不同、F11 缓存失效不同 | 统一菜谱领域写命令 |
| handler 直接写 users 安全字段 | F06 错误与并发契约缺失 | 认证服务集中维护安全状态 |
| 全局 database.DB 被各层直接引用 | F05 难统一 context；F09 难共享事务 | 显式传 context/事务句柄，不必为每张表造复杂抽象 |
| 模型清单在启动、迁移、注销多处手工枚举 | F18 已漏迁移表；家庭解散也遗漏派生清单/申请清理 | 注册表/契约测试验证清单职责，删除策略显式化 |
| UI 身份、localStorage token、Query cache 独立 | F07 多页错位 | session epoch 统一关联 |

没有将“大文件”或“缺少注释”单独列为问题。真正需要调整的是已经发生漂移的规则所有权，不建议为了目录形式全面重写。

## 10. Release Concerns

当前发布能够健康启动，线上镜像采用非 root、最小 capability、模型只读挂载、依赖 lock/hash；这些应保留。未在仓库看到完整的自动化测试与依赖审计门禁，外部 CI 是否配置本轮没有核验，因此不把“没有 CI”当作已确认事实。

下次发布前需要优先完成 F01/F06 与资源边界，修正 F16 的排空关系。迁移工具 F18 必须在下一次数据迁移之前补齐。线上版本在本次审查期间从 0.12.3 更新到 0.12.5，报告已据最终配置修订，没有继续沿用“尚未设置 GOMEMLIMIT”的旧结论。

## 11. Principles Compliance

### Principles Violated

下表数字是关联 issue 数，跨行可以重复，不应相加当作发现总数。

| Principle | Violations | Severity | Affected Areas |
|---|---:|---|---|
| Fail-Fast / 明确错误 | 3 | High/Medium | F05、F06、F09 |
| 原子性与单调安全状态 | 3 | High/Medium | F06、F09、F10 |
| 单一规则来源 | 3 | High/Medium | F02、F11、F18 |
| 有界资源与反压 | 7 | High/Medium | F01、F02、F03、F04、F12、F13、F15 |
| 状态归属与生命周期 | 3 | Medium | F07、F11、F14 |

### Principles Respected

用户/家庭可见性通过共享 scope 管理；PAT 只存哈希并有权限校验；AI 日配额与共享 lease 使用数据库原子条件；视频外部 I/O 与不可信解码有明确边界；新图片上传逻辑先准入后读 body，等待队列有上限；客户端流式结果要求完整结果与结束信号。

## 12. Fallback / Defensive Code Analysis

| Subtype | 关联问题/路径 | 决策 |
|---|---|---|
| SilentFallback / 忽略错误 | F06 安全更新、F09 记餐/清单 | FailFast；事务失败必须影响响应 |
| SilentCorrection | 超大正整数设置直接保留；部分业务默认值 | F03 先验证，再对旧数据显式迁移；不能静默接受危险值 |
| CompatibilityBranch | 周菜单旧 JSON 字符串/数组适配 | 保留，局部且有兼容目的；不是删除目标 |
| KeepWithAlert | 无字幕时返回可粘贴文稿；模型失败明确告知核对已保存结果 | 保留，补原因码/指标 |
| KeepWithAlert | 图片压缩失败时保存原图 | 当前有格式嗅探，但不是绕过 F01 解码风险的方案；升级依赖后维持可观测性 |

降级策略应说明失败状态，不能制造“安全操作已完成”或“全部采纳成功”的假象。模型审核失败后不执行未经审核结果的做法应保留。

## 13. Testing Authenticity Analysis

### Confidence Assessment

| Test Area | Real Confidence | Risk | Action |
|---|---|---|---|
| JWT/PAT、租户隔离、AI 额度 | High（已覆盖场景） | 部分安全写失败、跨请求旧状态仍逃逸 | 保留并加入 F06 |
| 上传字节/像素/排队 | High（正常与现有边界） | 依赖内部复杂格式漏洞无法仅靠尺寸测试发现 | 加官方解码回归样本 |
| 视频证据引用/数量/时间 | Medium–High（离线） | 真实字幕/ASR 质量及供应商输出未在线验证 | 保留；预发固定语料验证 |
| 草稿/UTF-8/SSE | High（模块环境） | 多标签页、真实微信底层仍有环境差异 | 保留；加入浏览器/微信演练 |
| DB 并发与故障 | Low | race detector 不检查业务可串行化 | 加隔离 MySQL 故障测试 |
| 部署/恢复/迁移 | Low | 成功启动与逐表完整迁移是不同保证 | 加排空与迁移演练 |

### Valuable Tests

已有真实路由隔离、限额边界、上传等待、分块 UTF-8、草稿恢复与错误响应测试具备回归价值，不能因为本次发现其他问题而否定这些测试。

### Suspicious Tests

未确认有应删除的“假测试”。需要注意 SQLite 单进程测试与 Node/JSDOM mock 的环境边界：其结果不自动代表 MySQL 并发锁语义或真实浏览器多页行为。

### Missing Tests

优先将本次 9 个 Go 和 2 个 Web 复现转为修复后的回归；再补 MySQL 交错、SMTP 半开、Linux ASR 资源峰值、双实例、SIGTERM 排空与迁移逐表核验。

## 14. Type Safety Analysis

| Subtype | 关联问题 | 判断 |
|---|---|---|
| InputBoundary / StringlyTyped | F03、F04 | settings 的 map[string]string 未转成受限领域值；普通 JSON 的外层容量无界 |
| ErrorType | F05、F06、F09 | error 被丢弃造成语义错误，比类型断言风格更优先 |
| OutputLeak | F19 | SQL 参数进入日志，需输出边界脱敏 |
| 动态 JSON 兼容 | 菜谱 JSON 字符串、对象、数组适配 | 已有校验与测试；逐步定义统一 DTO，避免无必要的大迁移 |
| UnsafeBlock / BooleanTrap | 未确认新增实锤问题 | 不因关键字或类型断言数量自行扣分 |

## 15. Frontend State Analysis

| Subtype | 关联问题/组件 | 结论 |
|---|---|---|
| StateDuplication | F07：useAuthStore / localStorage / Query | 必须共享身份世代，而不只是共享 token |
| RequestState | F07：Account mutation / Axios response | 旧成功响应仍需阻断 |
| SessionInjection | F08：WebView fragment bridge | 以一次性交换绑定登录流程 |
| Draft lifecycle | Web/miniprogram 编辑器 | 已有用户/菜谱隔离与卸载保存验证，保留 |
| Stream lifecycle | 聊天/视频提炼 | 完整消息、UTF-8 分片、取消/旧 token 测试已存在 |

## 16. Backend API Analysis

| Subtype | Finding | API / Service |
|---|---|---|
| Validation | F02–F04 | 创建/复制菜谱、settings、普通 JSON |
| Auth | F06、F10 | 改密/撤销、家庭成员变更 |
| Caching | F11 | 周菜单与菜谱变更 |
| ErrorResponse / BusinessLogic | F09 | 记餐、建议采纳、家庭菜单清单 |
| DataFlow / Cancellation | F05 | UserAuth、全局 DB、healthz |
| N+1 / 全量查询 | F02、F12 | 随机菜库、成就；FamilyPlan 逐项读菜谱也存在额外查询，但一周槽位有界，当前优先级低 |

## 17. Dependency Weight Analysis

### Dependency Scoreboard

| Dependency | Status | Weight / Transitives | Used For | Recommended Action |
|---|---|---|---|---|
| x/image v0.41.0 | 已确认需修复 | 未单独做二进制占比归因 | WebP 等图片解码 | 至少 v0.45.0，见 F01 |
| x/text v0.37.0 | 命中 GO-2026-5970，具体外部触发未确认 | 间接依赖 | Unicode/ORM 等 | 升至至少 v0.39.0 并回归 |
| x/net v0.51.0 | 命中 GO-2026-5026 | 间接依赖 | HTTP/IDNA | 升至至少 v0.55.0；没有据此断言 SSRF 绕过 |
| quic-go v0.59.0 | 命中 GO-2026-5676，但应用未启用 HTTP/3 服务 | Gin 间接带入 | 协议支持 | 升依赖清理；不列为当前线上可利用漏洞 |
| React/Router/Query/Axios 等 | 本次 npm audit 未报已知漏洞 | 294 总依赖项（含开发/可选）；50 prod 分类 | Web 客户端 | 保留；持续扫描 |
| sherpa-onnx / numpy / FFmpeg | 已固定 Python 依赖 hash；系统包未做完整 CVE/SBOM 扫描 | 模型主文件约 228 MiB；不等于加载峰值 | 本地 ASR | 保留锁定；补镜像 SBOM 和 native 回归 |
| SQLite 驱动 | 仍用于本地/迁移/测试 | 未证明可无损删除 | SQLite 兼容 | 不为减少依赖而直接移除 |

前端构建入口 JS 约 **494.34 kB，gzip 168.04 kB**，子页面已 lazy load；未据此判定前端体积是当前线上首要瓶颈。

**扫描结果的适用性过滤：**

1. 扫描输出使用本地 Go 1.26.5；GO-2026-6218、6090、6089、5972 及 5026 的标准库部分，在 Go 1.25 分支的修复版本为 1.25.13，线上是 **1.25.14**，不计作线上未修复项。本地开发工具链仍应更新。
2. 32 位 WebP 告警不适用于线上 x86_64；TIFF 在直接上传格式白名单之外，没有确认可绕过嗅探的入口。
3. HTTP/3 的静态调用图告警不代表当前应用启动了 HTTP/3；未将它宣传为公网实锤。
4. F01 的 WebP 是白名单内格式，线上确实链接了受影响版本，因此优先级高于上述间接/条件告警。
5. 公告原文、版本区间和未经删改的 server 扫描结果保存在证据目录，供复核。没有进行 OS 镜像全量漏洞扫描，不能声称镜像依赖已全部安全。

---

## 18. Recommended Fix Order

### Fix Immediately

| 顺序 | 问题 | 验收标准 |
|---|---|---|
| 1 | F01 | 发布二进制不再链接受影响的 x/image，合法图片和恶意样本回归通过 |
| 2 | F06 | DB 失败不返回安全操作成功；token version 不回退 |
| 3 | F02/F03 | 所有入口共享原子配额；每餐数量和输出规模有硬上界 |
| 4 | F04 | 普通 API body/并发/频率受控，改密与公共回源有限额 |
| 5 | F05 | 取消请求释放 DB 工作；healthz 有短超时 |
| 6 | F15 | 组合负载下重任务有预留与拒绝，宿主机不因应用峰值失守 |

### Fix Before Stable Release

F07/F08 客户端身份，F09/F10 事务与成员并发，F11 缓存失效，F13 邮件 deadline，F16 排空；F19 日志脱敏成本较低，建议一起完成。

### Schedule Later

F12 增量/异步成就、F14 完整对象生命周期、F17 多实例幂等。F18 在下一次迁移之前必须完成，若近期无迁移计划可晚于在线风险。

### Ignore for Now

没有证据支持现在就上 Kubernetes、全面拆微服务、删除 SQLite 或重写前端。小型有界 N+1、文件行数、风格和注释数量不应挤占以上修复。

## 19. Quick Wins

| 改动 | 预估 | 收益与限制 |
|---|---|---|
| 菜单数量 API/读取双校验 | 2–4 小时 | 快速阻断 F03；仍需修 F02 和候选读取 |
| GORM 参数化日志 | 2–4 小时 | 降低故障时敏感参数落日志风险 |
| SMTP 会话 deadline | 2–4 小时 | 避免半开无限等待；后续再补共享额度 |
| healthz 的 PingContext | 1–2 小时 | 健康检查有边界；不能替代全链路 context |
| 补迁移模型清单契约测试 | 2–4 小时 | 发现漏表，避免工具“成功但不完整” |
| 统一检查安全写结果 | 2–4 小时 | 消除写失败假成功；并发版本单调性仍要单独修复 |

## 20. Long-term Refactor Plan

| 动机 | 方案 | 主要风险 | 测试策略 |
|---|---|---|---|
| F02/F06/F09/F10/F11 的规则分散 | 保留现有模块，统一少数领域写命令，显式 context/事务 | 改变旧接口的失败语义 | 路由契约、故障注入、MySQL 交错 |
| F12/F15/F16 的重任务与请求耦合 | 有界 worker；ASR 可独立容器；必要时持久 job ID | 重试、配额、幂等和取消协议增加复杂度 | 组合峰值、SIGTERM、任务重放 |
| F11/F12 的完整快照与全量重算 | ID/版本缓存、增量投影、后台可重建 | 旧缓存兼容与失效覆盖 | 双版本缓存、事件乱序/重复、重建一致性 |
| F14/F18 的数据生命周期漂移 | 对象引用账本、模型清单契约、恢复演练 | 共享图片误删、迁移遗漏 | 哨兵数据迁移、引用计数、恢复后关联核验 |

以上调整均可分阶段实施，无需停止现有功能或开展全项目重写。
