# 每日菜谱推荐

推荐算法版本为 `personal-meal-v2`。首页、午晚餐、心情、明日菜单、周菜单、盲盒、Web 转盘候选、站内助手、MCP 与 Agent REST 使用同一推荐核心。

## 选菜与隐藏评分

先过滤当前用户不可见、停用、过敏与明确忌口冲突、请求排除食材/菜品，以及不满足餐次、必含食材、难度和请求时长的菜。未知耗时不能通过明确的时长上限。过敏、忌口和排除条件不能在无候选时自动取消；`ignore_preferences` 也不能绕过这些条件。

必含食材和家中库存使用真实原料身份及同义词；过敏排除另做分类展开。例如“西红柿”和“番茄”可互换，生抽库存不等于豆腐库存；“贝类”“坚果”等预置分类可排除对应常见原料。词表不是对未知复合配料的完整识别，菜谱原料应持续核对。

每道候选菜的六项得分均为 0–100，最终权重为个人喜好 35%、当餐匹配 20%、近期新鲜感 15%、做饭便利 15%、整餐搭配 10%、资料完整度 5%。个人喜好采用 5 个先验样本的平滑，真实评价按 45 天半衰期衰减；收藏与私房菜是有限先验，不再把整分乘以来源倍数。已吃、评价及反馈变更后可以从原始证据重算 `user_dish_affinities`，缓存不是原始事实。

星级优先于同一次记录的情绪评价，未评价记餐只产生弱证据；正负证据分别影响菜和画像。未来旧记录不参与当前实际食用画像，不自动推断或批量迁移历史含义不清的记录。新写入的未来餐次进入独立计划。

多道菜逐道重排，考虑主食材、菜品角色、做法和辣度。已有计划参与占位与搭配，同日尽量不重复，周内优先不重复。探索只从符合约束、分数接近且没有明确差评的新菜中选取，概率约 20%；`diversity=false` 时关闭探索与搭配加减分。菜谱结构来自原料及可识别特征，未知保留未知，不把历史“素菜”标签当作严格素食依据。

分数不代表概率、好吃程度或营养评价。用户界面只展示主要推荐理由；旧接口的 `score` 保留供现有客户端兼容。

## 每日稳定性与反馈

推荐按北京时间的日期、餐次、请求条件及用户证据生成轻量快照，只存菜 ID、评分分项和理由。普通重载复用结果；当前可见性、启用、禁忌和约束会重新核验。不同设备与来源可以复用同一用户的快照；同时首次请求通过用户行锁收敛。

返回新增 `recommendation_id`、`session_id`、`served_for_date`、`meal_type`、`algorithm_version`，每项增加 `item_id`。连续换菜传 `session_id`、`refresh=true`，已实际展示/跳过的菜在会话内排除；显式 `exclude_dish_ids` 永不被清空。快照保留 30 天，按用户限量清理过期项。

返回候选不等于真实曝光。页面可见才发送 `impression`；预取菜不直接计曝光。`skip` 是换一个，不降低长期偏好；`not_today` 当天冷却；`dislike` 是明确不喜欢，`undo_dislike` 撤销。旧 `reject` 仅保留短期弱信号。归因反馈校验用户与推荐菜的关系，`request_key` 防重，事件与幂等凭据同事务保存。通用 `/behavior` 不能绕过归因校验。

采纳率按去重后的真实曝光项及对应采纳项计算；缺少推荐关联的历史事件不纳入新指标。加入计划时由服务端记录采纳，客户端无需另发 `accept`。

## 计划与真实食用

复用 `health_plan_items` 管理个人已确认菜单。新增 `source` 和 `recommendation_id`，保留健康报告原接口。计划与购物条目同事务写入，添加幂等、整餐替换原子提交。撤销计划时仍有实际记录的采购条目保留，删除实际记录时仍有计划的条目也保留。

| 接口 | 用途 |
| --- | --- |
| `GET /api/meal-plans?date_from=…&date_to=…` | 查询已确认菜单 |
| `POST /api/meal-plans` | 添加一道计划菜 |
| `PUT /api/meal-plans` | 原子替换整餐，空 `dish_ids` 清空 |
| `DELETE /api/meal-plans/:id` | 撤销一道计划菜 |
| `POST /api/recommendation-feedback` | 可靠归因反馈 |

添加输入：`dish_id, meal_date, meal_type, recommendation_id?`。替换输入：`meal_date, meal_type, dish_ids, recommendation_ids?`，其中 `recommendation_ids` 以菜 ID 为键、推荐 ID 为值。手动选菜可不关联推荐；明确选用时允许改餐次或有效日期，服务端重新核验实际计划的权限和忌口。

计划允许今天起七天内、早餐/午餐/晚餐，每餐最多 10 道。计划不会写入 `meal_records`，实际吃过再记餐。旧站内 `/records`、批量接口和 Agent REST 的未来写入兼容转计划，并返回 `planned`；实际 `created` 只含真实记录。Agent 需要 `plan:write` 才能写未来计划。旧 MCP `log_meal` 的未来日期同样转计划，返回 `planned=true`、`plan_id`，不返回真实 `record_id`。

Agent 建议收件箱采纳也进入计划，保留响应的 `created` 数组，新增 `planned=true`。已有独立确认计划在周菜单重生成时保留。

## 第三方 MCP 与发布

Agent API 版本为 `2.1`，既有推荐参数和工具名保留，新增契约见 [Agent 接口](agent-api.md)。工具注册表同时生成 MCP、Function Calling 和 OpenAPI 定义，所有数据仍绑定令牌所属用户。

新增 `user_dish_affinities`、`recommendation_snapshots`、`recommendation_feedbacks`，并为 `health_plan_items` 添加两列。启动、离线迁移与账号注销共享对应模型；上线需要先备份并在隔离数据库验证迁移，再运行一次独立迁移，正常 API 保持 `DB_AUTO_MIGRATE=false`。不回写历史评分或历史用餐日期。
