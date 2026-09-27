# Fuck My Shit Mountain Audit Report

**Project:** arre食谱推荐小助手（arre-cook）
**Audit mode:** full
**Date:** 2026-09-27
**Reviewer:** Codex / GPT-6

---

## 1. Executive Summary

本轮检查 Web、原生小程序、Go API、数据范围、编辑流程、Agent、依赖和发布链路，并直接修复确认的问题。界面保留既有品牌与配色，统一请求状态和弹层；原定新增食谱自动草稿，后按用户追加需求加入关于我们、助手额度与内容安全。以下统计包含修复前发现的问题，当前状态在每项说明中标明。

关键路径已通过后端测试、客户端回归、浏览器操作和组件模拟。安全验证包含数据库持久限额、并发租约、凭证类型隔离、MCP 批量计费、提示词检查和审核前输出隔离。真实 CPA 模型预检发现 32 token 会截断审核结论，已改为 512 token，并要求正常结束的明确结论；食谱放行与无关请求拒绝均已实测。

仍需区分验证边界：浏览器和组件模拟不等同于 iOS/Android 微信真机；模型审核不能保证零误判或零绕过；外部凭证持有人主动转发仍依靠账号权限和总预算限制。部署和小程序上传结果以本次发布回执为准。

### Score Dashboard

```text
Security        ████████░░  8.0  A   服务端限额、内容审核与权限隔离已补齐，模型判断仍有边界
Stability       ████████░░  8.0  A   编辑失败保留、错误传播与并发测试通过，真实模型有超时风险
Performance     ████████░░  7.5  A   页面按需加载，主包约 494 kB，尚无真实负载性能基线
Testing         ████████░░  7.5  A   关键路径有行为回归，微信真机和完整负载测试仍缺失
Maintainability ███████░░░  7.0  A   公共逻辑已集中，两套客户端和较大处理模块仍需维护
Design          ████████░░  7.5  A   API 边界、弹层与请求组件有一致约束，跨端仍有重复实现
Release         ████████░░  8.0  A   有版本计算、友好通知、备份和回滚步骤，微信发布依赖平台审核
─────────────────────────────────────
Overall         ████████░░  7.6  A
```

各维度 0–10，越高越好；评分是基于本轮证据的工程判断，整体分为七项算术平均后保留一位小数，不代表形式化安全认证。

### Finding Statistics

| Severity | Count | Confirmed | Suspected |
|----------|-------|-----------|-----------|
| Critical | 0 | 0 | 0 |
| High | 3 | 3 | 0 |
| Medium | 6 | 6 | 0 |
| Low | 1 | 1 | 0 |
| Info | 1 | 1 | 0 |
| **Total** | **11** | **11** | **0** |

## 2. Project Map

- Web：React、TypeScript、Vite；路由在 `frontend/src/App.tsx`，请求在 `src/api`，登录与应用信息在 Zustand，服务端查询在 TanStack Query。
- 小程序：原生 JS/WXML/WXSS；页面共享 `api`、`session`、`sheet` 和主题工具，系统授权由微信处理。
- API：Go/Gin → 鉴权/参数中间件 → handlers → services → GORM；生产 MySQL，本地回归 SQLite。图片使用 Garage S3 或本地上传目录。
- AI：站内用户 JWT → 来源/参数/并发/每日预算 → 输入审核 → 有权限的食谱工具 → 完整输出审核 → SSE 正文/卡片；外部 Agent 用 PAT 或短期会话访问工具，不能使用站内模型接口。
- 发布：语义版本源 `VERSION`、`scripts/next-version.sh`、Docker Compose、启动通知、微信 `miniprogram-ci`；凭证、临时证据及私钥不进入仓库。

重点边界是账号归属、数据库写入前校验、异步表单状态、微信固定定位，以及模型输出和工具参数中的不可信内容。

## 3. Top Risks

1. **F01 / High / 已修复**：路径 ID 曾可能作为 SQL 表达式进入查询。
2. **F03 / High / 已修复**：站内模型缺少持久的每日使用预算和全站资源上限。
3. **F04 / High / 已修复**：不可信提示与模型正文缺少完整的输入、操作、输出审核边界。
4. **F06 / Medium / 已修复**：食谱长表单离开或失败后丢失输入。
5. **F07 / Medium / 已修复**：编辑格式转换可能破坏步骤图片、多行内容和复杂用量。
6. **F05 / Medium / 已修复**：小程序弹层与底部菜单、键盘之间缺少统一生命周期管理。
7. **F08 / Medium / 已修复**：请求失败、旧响应和分页截断影响用户看到的数据。
8. **F09 / Medium / 待真机验收**：自动化无法证明微信键盘、原生图片授权与渲染完全正确。

## 4. Detailed Findings

### Finding: F01 路径 ID 与更新参数边界

- Severity: High
- Confidence: High
- Category: Security
- Status: Confirmed；已修复
- Affected area: 菜谱、收藏、记录、管理与家庭接口
- Evidence: `backend/internal/middleware/path_ids.go:13`（`NumericPathIDs`）；`backend/internal/handlers/dish_validation.go:12`；`backend/internal/routes/ui_polish_test.go:16`。
- Problem: 将字符串 ID 直接交给 ORM 的主键捷径或动态更新，可能使表达式、负数、溢出及错误类型进入数据层。
- Why it matters: 破坏查询范围或已保存内容。
- Realistic failure scenario: 请求的 ID 是表达式字符串，或更新 steps 时传入错误结构。
- Minimal fix: 严格正整数路径检查、显式参数绑定和更新前字段校验，已完成。
- Better long-term fix: 保持每个新增接口的边界校验与同一响应约定。
- Regression test suggestion: 非法 ID 返回 400，错误更新后原菜谱内容不变；已有集成用例。
- Estimated effort: 已完成。

### Finding: F02 私房菜列表与分类范围不一致

- Severity: Medium
- Confidence: High
- Category: Stability
- Status: Confirmed；已修复
- Affected area: 我的私房菜、分类统计、编辑入口
- Evidence: `backend/internal/handlers/dish.go:22`、`backend/internal/handlers/dish.go:349`；`backend/internal/routes/ui_polish_test.go:70`。
- Problem: 个人入口和分类计数若复用全库范围，会让用户误以为自己的菜谱没有独立入口。
- Why it matters: 浏览、分类和编辑行为相互矛盾。
- Realistic failure scenario: 从“我的私房菜”进入后仍显示公共分类或公共菜谱编辑入口。
- Minimal fix: 列表和计数共同限定 owner 与个人范围，按实际权限显示编辑，已完成。
- Better long-term fix: 新列表沿用统一可见范围函数。
- Regression test suggestion: 两个账号及公共菜谱同时存在时，个人分类只计算本人个人菜谱。
- Estimated effort: 已完成。

### Finding: F03 模型资源缺少持久预算

- Severity: High
- Confidence: High
- Category: Security
- Status: Confirmed；已修复
- Affected area: 站内对话、MCP、Agent 凭证
- Evidence: `backend/internal/services/assistant_quota.go`（`ConsumeAssistantQuota`）；`assistant_lease.go`；`backend/internal/routes/ai_security_test.go`。
- Problem: 短期限流不足以控制全天、多个客户端或轮换凭证后的总消耗。
- Why it matters: 模型费用和服务容量可被持续占用。
- Realistic failure scenario: 重建对话、切换 Web/小程序或轮换 PAT 以增加可用次数。
- Minimal fix: 账号/全站事务计数、数据库租约、认证前 IP 限流、外部按账号计费，已完成。
- Better long-term fix: 应用多副本时将每分钟限流迁入共享存储；单实例部署继续使用当前实现。
- Regression test suggestion: 并发消费、跨连接、全站耗尽、旧 owner 释放、MCP 整批拒绝及令牌轮换；已有用例。
- Estimated effort: 当前范围已完成；多副本调整约 1–2 天。

### Finding: F04 提示词与未审核输出边界

- Severity: High
- Confidence: High
- Category: Security
- Status: Confirmed；已修复
- Affected area: 模型上下文、写入工具、流式事件
- Evidence: `backend/internal/assistant/guardrails.go`；`assistant.go`（`Run`、`runLLM`）；`guardrails_test.go`；`backend/internal/llm/limits_test.go`。
- Problem: 用户资料、菜谱、工具结果和模型事件 ID 都可能包含不可信文本；先流出再审核无法撤回内容。
- Why it matters: 可能展示越界内容、错误执行写入或泄露模型产生的文本。
- Realistic failure scenario: 查询做法时模型提出收藏操作，或将未审核文本放在工具事件 ID 中。
- Minimal fix: 固定系统角色、本地检查、模型审核、按当前提问筛选写工具、写入前复审、完整输出审核及服务端事件 ID，已完成。
- Better long-term fix: 持续收集最少量的拒绝阶段与用户反馈，针对真实误判补充用例。
- Regression test suggestion: BLOCK 不进入生成、输出未审核不流出、被拒写入不落库、截断 ALLOW 不接受；已有用例。
- Estimated effort: 已完成；后续规则维护按实际样例进行。

### Finding: F05 昵称弹层与底部菜单重叠

- Severity: Medium
- Confidence: High
- Category: Stability
- Status: Confirmed；实现与模拟回归完成，真机见 F09
- Affected area: 小程序 sheet、昵称、键盘；Web 弹层焦点
- Evidence: `miniprogram/components/sheet/index.js:38`、`miniprogram/utils/sheet-tabs.js:5`；`scripts/tests/nickname-editor.test.cjs`；`frontend/src/lib/use-dialog.ts`。
- Problem: 底部菜单恢复时机、页面离开和重复开关不同步，毛玻璃又降低按钮可读性。
- Why it matters: 保存按钮被挡住或无法操作。
- Realistic failure scenario: iOS 键盘弹出时打开昵称，快速关闭重开或离开页面。
- Minimal fix: 实底表单、统一键盘高度、真实 tab 组件锁和关闭完成后恢复；Web 管理焦点与 Escape。
- Better long-term fix: 新表单只复用公共弹层，避免页面各自操作 tab 显隐。
- Regression test suggestion: 成功/失败保存、快速重开、遮罩关闭、卸载恢复；已有 6 项行为用例。
- Estimated effort: 已完成；真机验收约 30–60 分钟。

### Finding: F06 食谱编辑中断后输入丢失

- Severity: Medium
- Confidence: High
- Category: Stability
- Status: Confirmed；已修复并作为原定新增功能交付
- Affected area: 两端新建与修改菜谱
- Evidence: `frontend/src/lib/use-recipe-draft.ts:4`、`frontend/src/lib/recipe-draft.ts`、`miniprogram/utils/recipe-draft.js`、`scripts/tests/recipe-drafts.test.cjs`。
- Problem: 长表单误返回、切后台或保存失败后缺少恢复路径。
- Why it matters: 用户需重新输入完整食谱。
- Realistic failure scenario: 粘贴较长步骤后立即离开，或网络失败后返回列表。
- Minimal fix: 450 ms 自动保存、离开立即写入、恢复前阻止覆盖、失败保留与成功清理，已完成。
- Better long-term fix: 保持本地草稿的账号/菜谱隔离，暂不引入跨设备同步复杂度。
- Regression test suggestion: 立即卸载、恢复/丢弃、跨账号、保存后卸载、空间不足；已有回归。
- Estimated effort: 已完成。

### Finding: F07 菜谱文本转换丢失结构

- Severity: Medium
- Confidence: High
- Category: Stability
- Status: Confirmed；已修复
- Affected area: 食材用量、多行步骤、步骤图片、零分钟
- Evidence: `frontend/src/lib/recipe-text.ts`、`miniprogram/utils/recipe-text.js`；`scripts/tests/recipe-drafts.test.cjs`。
- Problem: 简单拆行和分隔符解析会丢失已有元数据。
- Why it matters: 修改一个无关字段也可能损坏完整做法。
- Realistic failure scenario: 只修改菜名后，图片步骤或带空格的用量发生变化。
- Minimal fix: 集中转换规则，未修改文本保留原始结构并保存零值，已完成。
- Better long-term fix: 后续若改成结构化编辑器，继续以这些保留行为为验收标准。
- Regression test suggestion: 两端编辑同一份带图片、多行与复杂用量的菜谱，保存后逐项核对；已自动化验证。
- Estimated effort: 已完成。

### Finding: F08 请求失败、分页和异步状态覆盖

- Severity: Medium
- Confidence: High
- Category: Stability
- Status: Confirmed；已修复
- Affected area: 公共请求状态、偏好、日历、账号、通知详情
- Evidence: `frontend/src/components/RequestState.tsx:4`、`frontend/src/pages/Preferences.tsx:49`、`miniprogram/utils/records.js`、`frontend/src/pages/Notifications.tsx:52`。
- Problem: 失败被当成空结果，旧账号/后台刷新覆盖当前状态，日历只读取第一页，通知详情可重复打开。
- Why it matters: 用户看到遗漏或过期数据，未保存输入被替换。
- Realistic failure scenario: 修改偏好期间触发窗口可见性刷新，或一个月记录超过单页容量。
- Minimal fix: 显式加载/失败/重试、dirty 保护、请求身份检查、读取全部日期范围分页及单层详情，已完成。
- Better long-term fix: 后续异步页面沿用公共请求组件与账号查询键。
- Regression test suggestion: 后台刷新、切账号、第二页记录、失败重试与多次打开详情；已有测试和浏览器回归。
- Estimated effort: 已完成。

### Finding: F09 微信真机验收缺口

- Severity: Medium
- Confidence: High
- Category: Testing
- Status: Confirmed；待体验版真机验收
- Affected area: iOS/Android 微信原生渲染、键盘、相机和图片权限
- Evidence: `docs/miniprogram.md`；本轮小程序证据来自组件模拟，未连接实体手机。
- Problem: 模拟器没有完整实现微信原生组件与系统授权行为。
- Why it matters: 不能据此宣布 iOS 样式、头像或相机操作已在真机通过。
- Realistic failure scenario: 手机安全区、软键盘高度或用户拒绝相册权限与模拟行为不同。
- Minimal fix: 上传同版本后用体验版逐项核验昵称弹层、头像、相机/相册、返回和上传。
- Better long-term fix: 发布流程加入固定的两类实体设备检查记录。
- Regression test suggestion: 明确记录设备、微信版本、系统版本、权限状态和操作结果。
- Estimated effort: 每次发布约 30–60 分钟。

### Finding: F10 双端维护与较大处理文件

- Severity: Low
- Confidence: High
- Category: Maintainability
- Status: Confirmed；公共逻辑已集中，保留局部技术债
- Affected area: 两端编辑与聊天、Agent handlers
- Evidence: `frontend/src/pages/admin/DishEdit.tsx` 约 458 行、`miniprogram/pages/chat/chat.js` 约 462 行、`backend/internal/handlers/agent.go` 约 767 行。
- Problem: 双端原生实现需要同步文案和协议，较大模块增加修改成本。
- Why it matters: 后续改动容易只更新一端或混合不相关职责。
- Realistic failure scenario: 协议内容、草稿转换或 SSE 字段只在一个客户端更新。
- Minimal fix: 公共工具、协议一致性测试与本轮 UI 指南，已完成。
- Better long-term fix: 按真实变更需求逐步拆分 Agent 接口与聊天传输层，保留行为测试后再移动代码。
- Regression test suggestion: 跨端协议/格式一致性、现有路由与流式行为回归。
- Estimated effort: 后续每个模块约 1–2 天。

### Finding: F11 性能证据覆盖有限

- Severity: Info
- Confidence: High
- Category: Performance
- Status: Confirmed；已记录边界
- Affected area: 首屏资源、真实模型时延与大数据量
- Evidence: Vite 构建主包约 493.68 kB（gzip 167.77 kB）；`frontend/src/layouts/MainLayout.tsx` 按需加载；实际 CPA 审核样本约 2–10 秒。
- Problem: 本轮有资源大小和功能验证，没有真实流量压力测试或移动网络分位时延基线。
- Why it matters: 不能据此承诺大并发和慢网络性能。
- Realistic failure scenario: 模型响应接近 20 秒审核超时，用户看到忙碌或重试提示。
- Minimal fix: 已限制全站并发、总预算、响应大小与超时，并提供进度与友好失败状态。
- Better long-term fix: 有实际流量后记录匿名时延分布，再针对瓶颈拆包或调优。
- Regression test suggestion: 使用独立测试环境评估首屏、长列表和真实模型时延，避免对生产压测。
- Estimated effort: 有代表性数据后约 1 天。

## 5. Security Concerns

F01、F03、F04 已修复并有专门回归。JWT 必须有过期时间，未来签发时间被拒绝；PAT 不能访问站内模型。审核的失败、截断和未知格式按拒绝处理。第三方 Origin 限制不替代认证，凭证转发不能获得额外每日容量；没有“完全防反代”的保证。

## 6. Stability Concerns

F02、F05–F08 已处理。MySQL `condition` 保留字查询沿用此前修复，`findAutoAchievements` 使用列条件而非拼接保留字。额度数据库错误不会开放无限使用。AI 写入若已成功但最终回复失败，页面提示先检查对应记录，避免用户盲目重试造成重复。

## 7. Performance Concerns

页面按需加载、流式输入与输出大小都有边界。模型正文经过完整审核后再返回，首字时间会增加，页面用固定进度反馈。全站共享计数行的事务锁适用于当前预算；没有证据表明需要立即引入额外存储系统。F11 记录尚未测量的真实负载边界。

## 8. Testing Gaps

- Go 全量测试、`go vet` 通过；服务/助手/入口关键并发用例通过 race 检查。
- 客户端 21 项行为测试通过；Web lint 无问题、生产构建通过，npm audit 在官方 registry 返回 0 个已知漏洞。
- Web 主扫描 31 路由 × 3 布局 = 93，补扫 10 页面 × 3 = 30，均无检测到的横向溢出或运行时错误。布局为 320 px 浅色、390 px 深色、1440 px 桌面。
- 小程序主扫描 24 页 × 4 组合 = 96，补扫 4 页 × 4 = 16，均无模板错误或检测到的横向溢出；组合为 320/390 px × 浅色/深色。
- 浏览器真实操作 10 项核心流程与 8 项助手/关于我们流程通过。新 SSE 协议、全站暂停及跨端额度另有专门回归。
- 真实模型审核预检验证食谱与无关问题；完整生产链路在部署后做有限请求核验。未验证微信实体设备，不虚构覆盖率或渗透测试结论。

临时证据位于忽略目录 `dist/ui-polish-qa/`，不作为公开源码提交。后续接手者应使用仓库内测试和文档重新验证，不依赖这些本地文件永久存在。

## 9. Maintainability Concerns

F10 为剩余局部技术债。两端格式转换、协议文案和草稿都有一致性测试；组件和安全规则的来源分别在 UI 指南和助手安全文档中。短期内拆分文件应围绕实际需求进行，避免只按行数机械迁移。

## 10. Release Concerns

本轮包含新能力，按规则由 v0.9.5 递增至 v0.10.0。先提交实现，再计算、提交版本；发布摘要描述用户变化，站内信以 `vX.Y.Z` 展示，重复启动不重复发送。生产发布必须先备份 MySQL 和保留旧镜像，核对 revision、健康与通知，再上传同版本小程序。上传开发版本不等于微信审核通过或正式发布。

---

## 11. Principles Compliance

### Principles Violated

| Principle | Violations | Severity | Affected Areas |
|-----------|------------|----------|----------------|
| Single Responsibility (SRP) | F10 一组 | Low | Agent handlers、聊天与编辑页面 |
| File Size Limit | 3 个主要模块 | Low | F10 列出的文件 |
| Fail-Fast | F01/F03/F04，已修复 | High | 参数、额度与模型审核边界 |
| State Ownership | F05/F08，已修复 | Medium | 弹层、账号与异步表单 |

### Principles Respected

用户归属来自鉴权上下文；数据库预算与租约是服务端事实；表单由当前编辑状态控制；工具注册表统一约束权限；公共 UI 状态和格式转换集中维护。

## 12. Fallback / Defensive Code Analysis

### Fallback Summary

统计仅指本轮重点复核的路径，不是全仓库所有 catch 的数量。

| Subtype | Count | KeepWithAlert | FailFast | Remove |
|---------|-------|---------------|----------|--------|
| SilentFallback | 2 | 1 | 1 | 0 |
| EmptyCatch | 1 | 1 | 0 | 0 |
| CompatibilityBranch | 1 | 1 | 0 | 0 |
| SilentCorrection | 1 | 0 | 1 | 0 |
| DefensiveGuess | 0 | 0 | 0 | 0 |

- 未配置模型：保留本地食谱推荐，页面明确说明能力范围；上游失败不伪装成功写入。
- 数据库读取失败：偏好严格读取、额度查询与必要数据加载返回错误，防止静默使用空结果。
- 本地存储失败：草稿仍保留在当前表单，并提示无法自动保存；不能声称已持久保存。
- 旧嵌入地址字段：后端保留兼容，页面入口已移除，文档同步。
- 错误菜谱结构：更新前拒绝，已保存内容不被自动改成空结构。

## 13. Testing Authenticity Analysis

### Confidence Assessment

| Test Area | Real Confidence | Risk | Action |
|-----------|---------------|------|--------|
| Go API/隔离/额度事务 | High | SQLite 与生产 MySQL 行为差异 | Keep；部署后核对 MySQL |
| 浏览器真实填写/返回/失败重试 | High | 不能代替微信原生键盘 | Keep |
| 小程序 VM/组件模拟 | Medium | 原生组件和系统授权不完整 | Keep；补真机 |
| 模型 stub | Medium | 只能验证执行顺序与泄漏边界 | Keep；结合真实模型预检 |
| 页面溢出扫描 | Medium | 不证明所有视觉细节和操作正确 | Keep |

### Valuable Tests

跨用户权限、错误更新不破坏原数据、原子消费与租约接管、未审核内容不输出、禁止未授权写入、上传失败后草稿恢复，以及真实 DOM 焦点测试。

### Suspicious Tests

本轮未用截图存在、状态设置成功或编译通过替代行为测试。模拟器截图不能用于宣称微信像素验收通过；模型 stub 的 ALLOW 不能证明真实模型理解正确。

### Missing Tests

F09 微信真机、F11 真实负载基线；当前没有端到端的实体设备自动上传后验收。

## 14. Type Safety Analysis

### Summary

统计按本轮确认的问题组，交叉类别不重复计入总发现数。

| Subtype | Count | Critical | High | Medium | Low |
|---------|-------|----------|------|--------|-----|
| InputBoundary | 1 | 0 | 1 | 0 | 0 |
| OutputLeak | 1 | 0 | 1 | 0 | 0 |
| StringlyTyped | 1 | 0 | 0 | 1 | 0 |
| ErrorType | 1 | 0 | 0 | 1 | 0 |

F01 覆盖路径和更新边界，F04 覆盖未审核事件，F07 覆盖格式转换，F08 覆盖失败状态。TypeScript 构建通过；不将 Go map/断言的存在本身视为漏洞，没有声明全仓库达到零不安全断言。

## 15. Frontend State Analysis

### Summary

| Subtype | Count | Affected Components |
|---------|-------|-------------------|
| ComponentSize | F10 一组 | 编辑、聊天 |
| StateDuplication | F05/F08 两组，已修复 | tab 显隐、资料、额度 |
| RequestState | F08 一组，已修复 | 加载/失败/重试、日历、偏好 |
| UIBusinessCoupling | F06/F07 两组，已抽取 | 草稿与菜谱转换 |

草稿成功标记避免卸载重建；额度流事件优先于较旧状态请求；通知详情只有一个 selected 状态。尚未编辑的远程数据与正在输入的表单分离。

## 16. Backend API Analysis

### Summary

| Subtype | Count | Affected Endpoints |
|---------|-------|-------------------|
| Validation | F01 一组，已修复 | 带 ID 路径、菜谱更新 |
| Auth | F02/F03 两组，已修复 | 私房菜、站内模型与外部 Agent |
| BusinessLogic | F03/F04 两组，已修复 | 预算、批量、写入与审核 |
| ErrorResponse | F08 一组，已修复 | 数据库错误与分页 |

42901 为每日额度，42902 为并发繁忙；SSE 前的拒绝返回 JSON，已受理后的错误返回 error/done。外部批量预算不足不执行部分写入。

## 17. Dependency Weight Analysis

### Dependency Scoreboard

| Dependency | Status | Weight | Transitives | Used For | Recommended Action |
|------------|--------|--------|-------------|----------|-------------------|
| React / Router / Query | Healthy | 包含在约 494 kB 主包中 | 本轮未逐包测量 | 页面、路由与查询状态 | Keep，路由按需加载 |
| GSAP | Healthy | 本轮未单独分离统计 | 未逐包测量 | 既有界面动画 | Keep，避免增加整屏 transform |
| Vite 8.3.1 | Healthy | 构建依赖，不运行于服务端 | npm audit 已扫描锁文件 | 构建 | Keep |
| jsdom 30 | Healthy | 仅开发测试依赖 | npm audit 已扫描锁文件 | DOM/草稿回归 | Keep |
| 旧 AiAssistant iframe 组件 | Unused，已移除 | 不再进入构建 | 不适用 | 历史外部嵌入 | Remove，已完成 |

兼容范围升级后官方 npm registry audit 返回 0 个已知漏洞。该结果不是未来零漏洞保证，也不包含未执行的全生态供应链认证。

---

## 18. Recommended Fix Order

### Fix Immediately

F01/F03/F04 已完成；本轮没有已确认而故意遗留的高严重度代码问题。

### Fix Before Stable Release

生产完成备份、健康、MySQL、真实模型与通知核验；微信正式发布前完成 F09 体验版真机检查。

### Schedule Later

按实际修改需求处理 F10；有真实流量后建立 F11 性能基线；多副本前迁移进程内短期限流。

### Ignore for Now

保留技术标识 `ninimenu`、兼容字段及未影响行为的历史文件名；不以重命名扩大本轮迁移风险。

## 19. Quick Wins

已完成：统一产品名称、关于我们入口、单层通知详情、个人资料入口与头像同步、失败重试、私房菜范围、图文步骤保留、配额显示及停止后的输入恢复。

## 20. Long-term Refactor Plan

- **按职责拆分较大 Agent handlers**：在真实接口变更时抽取凭证、查询和工具调用模块；风险是路由/权限遗漏，使用当前隔离与作用域测试约束。
- **保留两端协议的一致性检查**：后续新增 SSE 或表单字段先定义契约，再分别实现；风险是平台 API 不同，继续用跨端数据测试和真机行为检查。
- **建立实际性能与模型误判样本**：只收集必要的匿名耗时和拒绝阶段，避免保存敏感原文；据样本做局部改进，不预先引入重型系统。
