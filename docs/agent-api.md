# 智能体开放接口（/api/agent/*）

食谱站把菜单、用餐记录、收藏、评价、行为事件、口味画像与推荐引擎全部开放给外部智能体，用于食谱推荐与用户行为分析。

## 凭证

所有 `/api/agent/*` 接口需携带令牌，三选一：

| 方式 | 示例 |
|---|---|
| 请求头 `X-Agent-Token` | `X-Agent-Token: <AGENT_TOKEN>` |
| `Authorization: Bearer <AGENT_TOKEN>` | 同上 |
| 管理端 JWT | `Authorization: Bearer <admin jwt>` |

`AGENT_TOKEN` 由环境变量控制，未设置时等于 `ADMIN_PASSWORD`（默认 `nini123`）。

响应结构与站内一致：`{ "code": 0, "message": "success", "data": … }`，非 0 为失败。

## 接口一览

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/agent/capabilities` | 能力清单：枚举值（菜系/口味/餐段/难度/心情/事件类型）、数据规模、全部端点 |
| GET | `/api/agent/dishes` | 菜品查询（完整字段含食材/调料/步骤）。参数：`category`、`taste`、`difficulty`、`meal_type`、`search`、`ingredient`、`exclude_ingredient`、`max_cook_time`、`exclude_recent=1`、`enabled`、`ids`、`limit`(≤500)、`offset` |
| GET | `/api/agent/dishes/:id` | 菜品详情 + 该菜用餐记录与统计（次数、好吃/一般/不行、均分、最近日期、浏览数、是否收藏） |
| GET | `/api/agent/profile?days=90` | 口味画像（见下） |
| POST | `/api/agent/recommend` | 推荐引擎（见下） |
| GET | `/api/agent/records` | 用餐记录，附菜系/口味/当天心情。参数：`date_from`、`date_to`、`meal_type`、`dish_id`、`limit`(≤2000)；默认最近 90 天 |
| POST | `/api/agent/records` | 写入用餐记录（单条或 `records` 数组），每条成功写入记一条 `accept` 事件并更新买菜清单 |
| DELETE | `/api/agent/records/:id` | 删除用餐记录 |
| GET | `/api/agent/favorites` | 收藏列表 |
| POST / DELETE | `/api/agent/favorites/:dishId` | 收藏 / 取消收藏 |
| GET | `/api/agent/behavior` | 行为事件流。参数：`type`(逗号分隔)、`source`、`dish_id`、`since`(日期或 RFC3339)、`limit`(≤2000) |
| POST | `/api/agent/behavior` | 写入行为事件（单条或 `events` 数组） |
| GET | `/api/agent/day-ratings` | 整餐评价与首页心情。参数：`date_from`、`date_to` |
| GET | `/api/agent/stats` | 整体统计（同管理端仪表盘） |
| GET | `/api/agent/week-plan` | 本周菜单 |
| POST | `/api/agent/week-plan/regenerate` | 重新生成本周菜单 |
| GET | `/api/agent/shopping-list` | 今明两日买菜清单 |
| GET | `/api/agent/settings` | 站点设置（分类、口味、去重天数等） |
| GET | `/api/agent/export?days=365` | 一次性导出菜品、记录、收藏、评价、事件与画像 |

站内应用端（`X-App-Token`）也新增了两个接口：`POST /api/pick/smart`（同推荐引擎，供首页与「AI 推荐官」页使用）、`POST /api/behavior`（前端埋点）、`GET /api/profile`。

## 口味画像 `GET /api/agent/profile`

由最近 `days` 天（默认 90，最大 365）的用餐记录、评价、收藏、首页心情、行为事件聚合；记录按时间衰减加权（45 天半衰期），好吃/高分加权、不行/低分降权。

```json
{
  "summary": "近 90 天记录了 38 餐、21 道不同的菜；偏好口味：香辣、麻辣、酸辣；常吃菜系：湘菜、川菜；辣味占比 81%；平均烹饪 28 分钟；收藏 6 道；近 3 天已吃过 4 道（推荐时避开）。",
  "window_days": 90, "repeat_days": 3,
  "total_records": 120, "window_records": 38, "distinct_dishes": 21,
  "meal_type_counts": {"lunch": 16, "dinner": 22},
  "taste_weights": [{"name": "香辣", "weight": 0.34, "count": 13}],
  "category_weights": [{"name": "湘菜", "weight": 0.41, "count": 15}],
  "difficulty_counts": {"easy": 12, "medium": 20, "hard": 3},
  "avg_cook_time": 28.4, "spicy_ratio": 0.81,
  "top_ingredients": [{"name": "鸡腿肉", "weight": 0.09, "count": 6}],
  "top_dishes": [{"dish_id": 1, "dish_name": "剁椒鱼头", "count": 5, "last_date": "2026-09-20"}],
  "recent_dish_ids": [1, 9, 41, 57],
  "last_eaten": {"1": "2026-09-20"},
  "favorite_dishes": [{"id": 9, "name": "辣子鸡", "category": "川菜", "taste": "香辣", "cook_time": 35, "difficulty": "medium"}],
  "liked_dishes": [], "disliked_dishes": [],
  "mood_counts": {"yum": 20, "ok": 6, "no": 2},
  "home_mood_counts": {"tired": 5, "spicy": 3},
  "behavior_counts": {"view": 40, "recommend": 24, "accept": 9, "reject": 6},
  "most_viewed_dishes": [{"dish_id": 8, "dish_name": "毛血旺", "count": 5, "last_date": ""}]
}
```

## 推荐引擎 `POST /api/agent/recommend`

所有字段可选；空请求 = 按画像给 3 道。

```json
{
  "meal_type": "dinner",
  "mood": "spicy",
  "count": 3,
  "keyword": "",
  "tastes": ["辣"],
  "categories": ["川菜", "湘菜"],
  "max_cook_time": 30,
  "difficulty": "easy",
  "include_ingredients": ["牛肉"],
  "exclude_ingredients": ["香菜"],
  "exclude_dish_ids": [12, 34],
  "exclude_recent_days": 3,
  "profile_days": 90,
  "diversity": true,
  "source": "agent:xxx",
  "actor": "user-1"
}
```

打分规则：画像口味/菜系权重、显式口味、收藏、喜恶评价、距上次食用天数（3 天内强降权、21 天以上加分、从未做过加分）、心情（tired/lazy 偏简单快手、spicy 必辣、healthy 偏清淡、happy 偏硬菜）、耗时与画像匹配度；候选为空时逐级放宽（先去口味硬过滤，再去近期去重，`applied.relaxed` 会说明）。`diversity=true` 时同一菜系最多占一半名额。每次推荐写入 `recommend` 行为事件。

响应：

```json
{
  "items": [{"dish": {…完整菜品…}, "score": 78.5, "reasons": ["常吃口味「香辣」", "有 12 天没吃了，换换口味"]}],
  "candidate_count": 37,
  "profile_summary": "…",
  "applied": {"meal_type": "dinner", "mood": "spicy", "count": 3, "exclude_recent_days": 3, "diversity": true, "profile_days": 90, "relaxed": []}
}
```

## 行为事件 `POST /api/agent/behavior`

```json
{ "event_type": "reject", "dish_id": 12, "dish_name": "夫妻肺片", "source": "agent:recipe", "actor": "user-1", "meta": {"note": "太麻了"} }
```

`event_type`：`view` 浏览、`recommend` 推荐、`accept` 采纳、`reject` 拒绝、`search` 搜索、`chat` 对话诉求、`feedback` 反馈、`custom`。站内前端会自动写 `view`（进入详情）、`accept`（记录用餐）、`reject`（首页「换一个」、推荐官「不想吃」）；推荐引擎自动写 `recommend`。`source` 用于区分来源：`app` 站内、`agent…` 智能体。
