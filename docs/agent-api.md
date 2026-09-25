# NiniMenu Agent 接口

NiniMenu 为 Hermes、DeepSeek Harness、自建 Agent 和兼容 MCP 的客户端提供个人数据接口。每个访问凭据绑定到一个用户；服务端从凭据解析用户身份，不接受调用方指定数据所有者。

## 创建访问凭据

登录 NiniMenu 后进入「我的 → AI 连接」，创建个人令牌并选择权限和有效期。令牌明文只在创建时显示一次，之后服务端只保存摘要。请将它作为密码保管，不要放进前端代码、公开仓库或客户端日志；泄露后立即撤销并重建。

| 预设 | 权限 | 适用场景 |
|---|---|---|
| `readonly` | `profile:read`、`dishes:read`、`records:read` | 查询画像、菜单和记录 |
| `advisor` | 只读权限，加 `suggestions:write`、`behavior:write` | 分析并提交待用户确认的建议 |
| `full` | 全部权限 | 允许 Agent 代记用餐、改偏好、收藏和重排周菜单 |

可单独选择的 scope：

| Scope | 能力 |
|---|---|
| `profile:read` | 读取口味画像、偏好和统计 |
| `dishes:read` | 读取公共菜谱和当前用户的私房菜 |
| `dishes:write` | 新建、修改、删除本人私房菜（不包含公共或家庭共享菜谱） |
| `records:read` | 读取用餐记录、饮食日记、报告、评分、收藏、行为、周菜单和购物清单 |
| `records:write` | 记录或删除用餐及手动饮食日记 |
| `favorites:write` | 收藏或取消收藏 |
| `plan:write` | 重新生成周菜单 |
| `preferences:write` | 修改饮食偏好 |
| `suggestions:write` | 向用户提交建议 |
| `behavior:write` | 写入行为反馈 |

所有 Agent 请求使用：

```http
Authorization: Bearer nm_个人令牌
```

也兼容 `X-Agent-Token: nm_个人令牌`。`/api/agent/*` 和 `/mcp` 只接受个人 Agent 令牌或短期嵌入会话令牌；站内登录 JWT 不能作为第三方凭证。旧的全站 `AGENT_TOKEN` 已移除。

每个 Hermes、DSH、DeepSeek Harness 或其他外部 Agent 都应由对应用户在「我的 → AI 连接」单独创建一个令牌。服务端按令牌绑定的 `user_id` 读取全部饮食隐私数据，不根据微信昵称、机器人名称或消息内容判断用户，因此不同机器人不能共享令牌。令牌泄露后应立即撤销或轮换；轮换会让旧令牌立即失效，新明文只显示一次。用户可以修改令牌名称、权限和有效期，也可以随时撤销。

## MCP

Streamable HTTP 地址为 `https://你的域名/mcp`。在 MCP 客户端配置该 URL 和 Bearer 令牌即可。以通用 JSON 配置为例：

```json
{
  "mcpServers": {
    "ninimenu": {
      "type": "http",
      "url": "https://cook.example.com/mcp",
      "headers": {
        "Authorization": "Bearer nm_个人令牌"
      }
    }
  }
}
```

服务端提供 `tools/list` 与 `tools/call`；MCP 和 `/api/agent/capabilities` 的工具清单都会按令牌权限过滤。每次调用都以令牌所属用户作为数据范围，并写入该用户的调用记录。

## 函数调用

`GET /api/agent/tools` 返回 OpenAI 兼容的工具定义，`POST /api/agent/tools/{name}` 的请求体是对应工具参数。`get_context` 包含今天已记录的餐，需要 `records:read`。统一响应为 `{ "code": 0, "message": "success", "data": ... }`。

```sh
curl -fsS 'https://cook.example.com/api/agent/tools' \
  -H 'Authorization: Bearer nm_个人令牌'

curl -fsS 'https://cook.example.com/api/agent/tools/recommend_dishes' \
  -H 'Authorization: Bearer nm_个人令牌' \
  -H 'Content-Type: application/json' \
  -d '{"meal_type":"dinner","mood":"spicy","count":3}'
```

饮食报告和日记工具：`list_food_journal`（`records:read`）、`log_food_journal` 与 `delete_food_journal`（`records:write`）、`get_health_report`（同时需要 `records:read`、`profile:read`、`dishes:read`）。私房菜管理工具：`create_private_recipe`、`update_private_recipe`、`delete_private_recipe`（`dishes:write`）；只处理令牌所属用户的私房菜。`get_context` 的“今天已吃”也包含手动日记。报告只整理已记录数据，不根据缺失记录推断未进食，不估算热量或给出医疗诊断。其余工具包括 `get_taste_profile`、`get_preferences`、`update_preferences`、`search_dishes`、`get_dish`、`recommend_dishes`、`list_meal_records`、`log_meal`、`rate_meal`、`delete_meal_record`、`list_favorites`、`set_favorite`、`get_week_plan`、`regenerate_week_plan`、`get_shopping_list`、`get_stats`、`list_behavior_events`、`log_feedback`、`create_suggestion`、`list_suggestions`、`get_day_ratings`。参数 schema 以实时工具清单为准。

例如使用只读个人令牌查看近 7 天报告：

```sh
curl -fsS 'https://cook.example.com/api/agent/tools/get_health_report' \
  -H 'Authorization: Bearer nm_个人令牌' \
  -H 'Content-Type: application/json' \
  -d '{"days":7}'
```

DeepSeek 等 OpenAI 兼容模型的推荐流程：把 `/api/agent/tools` 返回的 `data` 作为模型的 `tools`；收到 `tool_calls` 后，逐个 POST 到 `/api/agent/tools/{function.name}`，并把工具响应作为对应的 `tool` 消息交回模型。模型服务密钥由调用方自己的服务端持有，不能使用 NiniMenu 用户令牌替代。

## REST 接口

工具接口适合新集成；下列资源接口用于需要直接读写结构化数据的客户端。所有资源都只作用于当前凭据对应的用户。

| 方法 | 路径 | Scope |
|---|---|---|
| GET | `/api/agent/capabilities`、`/api/agent/me`、`/api/agent/tools` | 无额外 scope；返回身份、权限或授权后的工具清单 |
| GET | `/api/agent/dishes`、`/api/agent/dishes/:id` | `dishes:read` |
| POST | `/api/agent/recommend` | `dishes:read` + `profile:read` |
| GET | `/api/agent/profile`、`/api/agent/stats`、`/api/agent/settings`、`/api/agent/preferences` | `profile:read` |
| PUT | `/api/agent/preferences` | `preferences:write` |
| GET | `/api/agent/records`、`/api/agent/favorites`、`/api/agent/behavior`、`/api/agent/suggestions`、`/api/agent/day-ratings`、`/api/agent/week-plan`、`/api/agent/shopping-list` | `records:read` |
| POST / DELETE | `/api/agent/records`、`/api/agent/records/:id` | `records:write` |
| POST / DELETE | `/api/agent/favorites/:dishId` | `favorites:write` |
| POST | `/api/agent/behavior` | `behavior:write` |
| POST | `/api/agent/suggestions` | `suggestions:write` |
| POST | `/api/agent/week-plan/regenerate` | `plan:write` |
| GET | `/api/agent/export` | 同时需要 `records:read` 和 `profile:read` |

越权请求返回 HTTP 403。无效或撤销的令牌返回 HTTP 401。建议使用 `/api/agent/capabilities` 查询部署实例公布的能力和完整端点清单。

## 建议闭环

顾问型 Agent 使用 `create_suggestion` 提交菜品或周计划建议。用户在「建议收件箱」查看后选择采纳或忽略；只有用户采纳时才会写入对应的用餐记录。Agent 可用 `list_suggestions` 查看状态，后续建议可结合用户反馈调整。

## OpenAPI

`GET /api/agent/openapi.json` 提供 OpenAPI 3.1 文档，可导入支持 Bearer 鉴权的 Dify、Coze 或 GPT Actions。该文档描述工具调用接口；访问数据仍需使用用户个人令牌。
