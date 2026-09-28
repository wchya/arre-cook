# 2026-09-28 审计修复记录

本轮已实现原审计报告 F01–F19 的代码修复。**实现提交 `47b62b1` 和版本提交 `8085433` 已推送，0.12.7 于 2026-09-28 15:16（北京时间）完成生产部署，小程序同版本已上传微信后台开发版本。** 容器限制和独立迁移已生效，发布证据见 [0.12.7 发布记录](releases/0.12.7.md)。代码基线为 `3be2e6d` / 0.12.6，保留此前的抖音改动；线上升级前实际运行 0.12.5。历史 [审计报告](audit-report-arre-cook-2026-09-28.md) 和 [缺陷复现证据](audit-evidence/2026-09-28/README.md) 不改写为通过记录。

## 逐项状态

以下“通过”指各行列出的验证范围；生产健康、接口、容器配置及真实模型检查见发布记录，Linux 组合负载、真实视频平台调用与 iOS 真机仍另列。

| 编号 | 修复内容与代码 | 已完成验证 |
|---|---|---|
| F01 | [依赖](../backend/go.mod) 升级 x/image 0.45.0、x/text 0.41.0、x/net 0.55.0、quic-go 0.59.1；[Dockerfile](../Dockerfile) 固定 Go 1.25.14 | 上游 WebP 包测试通过；发布工具链 govulncheck：0 条可达漏洞 |
| F02 | [统一菜谱写入](../backend/internal/services/dish_quota.go)：个人与家庭最多 500 道，锁所属行并用当前读检查；创建、复制、分享、Agent 共用；随机列表先取 ID，搜索流式筛选再取分页详情 | 满额入口拒绝、499 道并发竞争、搜索契约；MySQL 两连接池持旧 RR 快照时仅一次插入成功 |
| F03 | [菜单](../backend/internal/services/plan_service.go)：API/读取双校验每餐 1–10、去重 1–30；旧非法菜数迁移为 1；加权随机排序取代反复全池扫描；最多 7 天；卡片字段有长度界限，缓存只存 ID | 超大/负数/溢出设置拒绝，旧设置不绕过；缓存不含完整菜谱，忌口仍检查调料 |
| F04 | [请求预算](../backend/internal/middleware/request_budget.go)：鉴权前 API/MCP 并发 16、IP 360/分钟；普通 JSON 64 KiB、菜谱/Agent/MCP 256 KiB及结构上限；密码计算并发 2、10/分钟；菜库/导出读取并发 2；[回源](../backend/internal/storage/store.go) 并发 8 | 超限普通请求返回 413；既有上传准入、认证、路由回归通过 |
| F05 | [DB 句柄](../backend/internal/database/handle.go) 显式传递 request context/事务；HTTP、Agent、助手全链路传递；普通请求 15 秒，AI 90 秒，上传/邮件 30 秒；MySQL 建连 5 秒、读写 15 秒，healthz 2 秒 | 取消请求不再鉴权读取；MySQL 查询取消约 100ms 返回、客户端连接池继续可用；不声称 MySQL 服务端瞬间停止所有已执行工作 |
| F06 | [安全写入](../backend/internal/handlers/auth.go)：改密按旧密码哈希、旧版本和启用状态 CAS，版本原子递增；撤销/禁用检查写入结果 | 写失败返回失败、不签虚假令牌；延迟改密不能回退版本或恢复旧令牌 |
| F07 | [Web 客户端](../frontend/src/api/client.ts) 标签页 token + session epoch；storage/focus 同步身份，发送前阻断旧表单，拦截迟到成功与失败；Account mutation 再检查世代 | 延迟 storage 事件、A→B→A、迟到改密回调、禁用浏览器存储等真实模块回归通过 |
| F08 | [旧桥接入口](../frontend/src/lib/miniprogram.ts) 不再消费 URL fragment 中的 token，只清除旧片段；`from=mp` 不能伪造运行环境；现小程序为原生登录页面 | 普通浏览器和伪造小程序来源链接均不能替换已有登录态 |
| F09 | [记餐](../backend/internal/services/record_service.go)、[建议采纳](../backend/internal/services/agent_service.go)、[家庭计划](../backend/internal/services/family_service.go) 与关联清单同事务；通知后置；建议条件更新、重试幂等 | 注入清单/记录写失败后整体回滚；删除回滚；采纳失败保持 pending；重复采纳不重复记餐 |
| F10 | [家庭服务](../backend/internal/services/family_service.go) 统一 family→member/user 锁顺序，当前读复查成员/管理员；注销复核成员状态；解散清理派生清单及删除申请 | 确定性交错回归；MySQL 转让/退出竞争 10 轮，均保留有效管理员 |
| F11 | [菜单缓存](../backend/internal/services/plan_service.go) 每次按当前可见性、启用状态、内容与忌口 hydrate；无需靠分散入口遗漏失效通知 | 更新菜名/食材、停用、改入过敏调料均即时反映；原缓存兼容/收藏/用户隔离测试通过 |
| F12 | [成就调度](../backend/internal/services/achievement_service.go)：128 等待用户、2 worker、按用户合并 dirty 状态；GET 只排队并读现有结果；不读取完整做法；唯一授奖约束 | 一万个用户触发不会产生无界队列；重复请求合并，溢出后可重试；原成就回归通过 |
| F13 | [SMTP](../backend/internal/mailer/mailer.go)：context、20 秒会话 deadline、建连 5 秒、取消关闭 socket、并发 2；验证码短事务预约共享额度，事务外发送，失败仍计额度 | 本地假 SMTP greeting 停滞可取消；认证/邮件服务回归通过，无真实邮件发送 |
| F14 | [图片账本](../backend/internal/database/uploads.go)：用户 256 MiB/1000 对象，站点 5 GiB/50000 对象；上传先预约、业务事务内绑定引用、24 小时未引用可回收；删除置墓碑，失败保留额度；[清点](../backend/internal/storage/inventory.go) 分页回填历史对象与备份 | pending 占额、共享/步骤/头像引用、业务回滚、删除后拒绝绑定、原图备份回收、解散后个人副本保护、S3 分页/重复游标/双存储副本；MySQL 旧快照不能绕过墓碑 |
| F15 | [重任务预算](../backend/internal/resourcebudget/heavy.go) 让图片解码与 ASR 共用单槽；[Compose](../docker-compose.yml) 配置 1 GiB/2 CPU、Go 192 MiB/并行度 2，保留 ASR 768 MiB 启动余量/512 MiB watchdog | 互斥准入、取消回归；Compose 静态校验通过；生产容器限制已核对生效，Linux 混部峰值尚未验证 |
| F16 | [服务排空](../backend/cmd/server/main.go)：停止接流后保留在途 request context，Shutdown 100 秒、容器 stop grace 110 秒；随后取消并等待重任务释放 | 构建/竞态/配置校验通过；执行中真实 ASR 的 Linux SIGTERM 演练尚未执行 |
| F17 | [共享窗口/任务键](../backend/internal/database/admission.go)、成就唯一键、发布通知分批且版本幂等；[迁移锁](../backend/internal/database/migration_lock.go) 固定连接串行初始化，短轮询不超驱动读期限；可独立迁移、API 跳过迁移 | MySQL 独立连接池并发限流和通知幂等通过；两个真实迁移进程在连接池=1时均成功，最终仅1管理员/100种子菜，账本标记 done |
| F18 | [模型注册表](../backend/internal/models/registry.go) 供启动与 [迁移工具](../backend/cmd/dbmigrate/main.go) 共用；流式读取支持复合主键/软删除记录，分批写并逐表核数 | 全部注册模型哨兵、多批读取、空表清空回归通过；含原遗漏的家庭清单、删除申请表 |
| F19 | [安全 SQL 日志](../backend/internal/database/logging.go)：参数化模板、剥离 driver 错误中的实际值，保留错误类型/码 | 真实唯一键失败与虚构敏感标记回归，日志不含标记 |

## iOS 小程序弹框

[我的页面](../miniprogram/pages/me/me.js) 修复去重天数与昵称底部弹框的打开/关闭动画状态、页面离开时复位及自定义 tabbar 同步；[样式](../miniprogram/pages/me/me.wxss) 固定背景与层级。4 个新增生命周期回归通过。0.12.7 已上传微信后台开发版本；iOS 微信真机仍需检查键盘、底部安全区和关闭动画，未提交平台审核或正式发布。

后续用户确认去重天数弹框仍与底部导航重叠，所以上述状态测试不能作为视觉问题已解决的证据。当前实现已将去重天数改为微信原生 `picker mode="selector"`，删除该项自定义 sheet 和导航隐藏状态；保留 1/2/3/5/7/10/14 天及默认选项，只在确认后保存，失败回退显示值和选中项。昵称继续使用原有弹层。新增原生选择事件、默认值、失败回退、重复确认回归，客户端总计 46 项通过；仍须用对应新包在 iOS 微信真机验收。

## 本地验证

[结构化测试记录与扫描输出](audit-evidence/2026-09-28/remediation/README.md) 保留在单独的 remediation 子目录，原缺陷复现断言仍作为历史证据。

- Go 1.25.14：全仓 `go test -race -count=1 -json ./...`，**273 个测试及子测试通过、3 个跳过**；包含隔离 MySQL 8.0.45 的六组子测试；`go vet ./...` 通过。
- 三项 Go 跳过为真实模型供应商、真实抖音元数据、Linux 本地 ASR 集成；未用线上凭据或真实流量验证。
- 客户端 `npm test`：**46 项通过**；lint、TypeScript/Vite 构建通过。
- Python ASR：6 项中 5 通过、1 个 Linux 沙箱测试跳过；抖音游客凭据脚本：3 项通过。
- 发布工具链扫描 `govulncheck ./cmd/server`：0 可达、0 已导入包漏洞；仍有 18 条位于所需模块但无可达调用路径的公告，不表示所有模块/OS 镜像都无公告。`npm audit` 官方 registry：0 漏洞。
- 上游 `golang.org/x/image/webp` 包测试通过。Compose 使用无秘密的占位环境静态校验通过，未构建/启动 Docker 镜像（本机 daemon 不可用）。
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` 发布二进制构建通过，核对 Go 1.25.14 / x/image 0.45.0；该二进制的 govulncheck 扫描同样为 0 可达漏洞。此检查不等于 Linux 运行时或完整容器镜像验收。
- 真实 MySQL 使用临时目录、独立 Unix socket、关闭 TCP；双进程迁移验证修正了迁移事务错用全局句柄导致的小连接池等待，并验证修复后两个进程退出码均为 0。

可复跑：

```sh
cd backend
GOTOOLCHAIN=go1.25.14 DB_DRIVER=sqlite go test -race -count=1 ./...
GOTOOLCHAIN=go1.25.14 go vet ./...
# 有临时 MySQL 实例时；socket 父目录必须以 arre-audit-mysql- 开头：
ARRE_AUDIT_MYSQL_SOCKET=/临时目录/arre-audit-mysql-example/mysql.sock \
  GOTOOLCHAIN=go1.25.14 DB_DRIVER=sqlite go test -race ./internal/services -run '^TestAuditMySQL$'
```

未提供 socket 时 MySQL 用例会明确 skip。扫描器 v1.8.0 自身要求 Go 1.26，先用相应工具链编译扫描器，再以 `GOTOOLCHAIN=go1.25.14` 运行该可执行文件扫描应用；不要把扫描器的编译版本误当应用发布版本。

## 发布边界

首次发布须按 [部署步骤](deployment.md#审计修复的首次迁移与发布) 备份、停写、完成引用/对象回填后启动 API；S3 密钥需要列举权限。新增配额不会自动清除已引用图片，历史备份无法确定对应关系时保守保留为 `archive`。定时 GC 每小时最多 100 个，删除失败继续计费并重试；注销不会删除其他菜谱仍引用的共享图片。

本机 MySQL 验证了真实锁/事务和多连接池行为；发布阶段另完成生产数据库副本恢复与迁移演练、停写备份、生产迁移和线上功能检查。仍未执行 Linux 混部峰值压测、完整灾难恢复、执行中 ASR 排空或 iOS 真机验收；此次发布检查未观察到容器 OOM 或重启。
