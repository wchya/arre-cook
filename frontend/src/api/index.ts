import { api, getToken, getUploadErrorMessage, errorMessage, ApiError } from "./client"
import defaultClient from "./client"
import type {
  Dish, DishInput, PaginatedData, PickResult, BlindBoxResult, MealRecord, DashboardData, Achievement, WeekPlan,
  ShoppingCategory, ShoppingCategoryOverride, Holiday, Quote, DayRating, PhotoWall, DishRecordsResponse, DishCategoryCounts,
  FavoriteOverview, SmartPickRequest, SmartPickResult, TasteProfile, BehaviorEventInput, AuthOptions, LoginResult, User,
  Preferences, UserStats, AgentTokenList, AgentToken, AgentAuditLog, AgentSession, Suggestion, ChatSession, ChatMessage,
  ChatCard, AssistantStatus, AdminUser, SiteSettings, AppInfo, FamilySnapshot, FamilyPlanItem,
  FamilyShoppingItem, FoodJournalEntry, FoodJournalInput, HealthReport,
  NotificationPage,
} from "@/types"

export { getUploadErrorMessage, errorMessage, ApiError }

export const appInfoApi = {
  get: () => api<AppInfo>("GET", "/app-info"),
}

export const authApi = {
  options: () => api<AuthOptions>("GET", "/auth/options"),
  sendEmailCode: (email: string) => api<{ cooldown: number; ttl_minutes: number }>("POST", "/auth/email/code", { email }),
  emailLogin: (email: string, code: string, wechatCode?: string) =>
    api<LoginResult>("POST", "/auth/email/login", { email, code, wechat_code: wechatCode }),
  passwordLogin: (account: string, password: string) => api<LoginResult>("POST", "/auth/login", { account, password }),
}

export const meApi = {
  get: () => api<User>("GET", "/me"),
  update: (data: { nickname?: string; avatar?: string }) => api<User>("PUT", "/me", data),
  changePassword: (data: { old_password?: string; new_password: string }) => api<LoginResult>("PUT", "/me/password", data),
  logoutAll: () => api<null>("POST", "/me/logout-all"),
  export: () => api<Record<string, unknown>>("GET", "/me/export"),
  remove: (confirm: string) => api<null>("DELETE", "/me", { confirm }),
  preferences: () => api<Preferences>("GET", "/me/preferences"),
  updatePreferences: (data: Partial<Preferences>) => api<Preferences>("PUT", "/me/preferences", data),
}

export const familyApi = {
  get: () => api<FamilySnapshot>("GET", "/family"),
  create: (name: string) => api<FamilySnapshot>("POST", "/family", { name }),
  rename: (name: string) => api<FamilySnapshot>("PATCH", "/family", { name }),
  remove: () => api<null>("DELETE", "/family", { confirm: "解散家庭" }),
  invite: (email: string) => api<{ id: number; email: string; link: string; sent: boolean }>("POST", "/family/invitations", { email }),
  revoke: (id: number) => api<null>("DELETE", `/family/invitations/${id}`),
  join: (token: string) => api<FamilySnapshot>("POST", "/family/join", { token }),
  transfer: (userId: number) => api<FamilySnapshot>("POST", "/family/transfer", { user_id: userId }),
  kick: (userId: number) => api<null>("DELETE", `/family/members/${userId}`),
  leave: () => api<null>("POST", "/family/leave"),
  shareDish: (id: number) => api<Dish>("POST", `/family/dishes/${id}/share`),
  plan: (start?: string) => api<FamilyPlanItem[]>("GET", "/family/plan", start ? { start } : undefined),
  setPlan: (mealDate: string, mealType: string, dishId: number) => api<null>("PUT", "/family/plan", { meal_date: mealDate, meal_type: mealType, dish_id: dishId }),
  shopping: () => api<FamilyShoppingItem[]>("GET", "/family/shopping"),
  addShopping: (name: string, amount: string) => api<FamilyShoppingItem[]>("POST", "/family/shopping", { name, amount }),
  checkShopping: (id: number, checked: boolean) => api<null>("PATCH", `/family/shopping/${id}`, { checked }),
  deleteShopping: (id: number) => api<null>("DELETE", `/family/shopping/${id}`),
  importIngredients: (dishId: number) => api<{ added: number }>("POST", "/family/shopping/import", { dish_id: dishId }),
}

export const healthApi = {
  journal: (from?: string, to?: string) => api<FoodJournalEntry[]>("GET", "/food-journal", { from: from || "", to: to || "" }),
  record: (input: FoodJournalInput) => api<FoodJournalEntry>("POST", "/food-journal", input),
  remove: (id: number) => api<null>("DELETE", `/food-journal/${id}`),
  report: (days: 7 | 30) => api<HealthReport>("GET", "/health-report", { days: String(days) }),
}

export const agentConnApi = {
  list: () => api<AgentTokenList>("GET", "/me/agent-tokens"),
  create: (data: { name: string; scopes: string[]; expires_in_days: number }) =>
    api<{ token: string; info: AgentToken }>("POST", "/me/agent-tokens", data),
  revoke: (id: number) => api<null>("DELETE", `/me/agent-tokens/${id}`),
  update: (id: number, data: { name?: string; scopes?: string[]; expires_in_days?: number }) => api<AgentToken>("PATCH", `/me/agent-tokens/${id}`, data),
  rotate: (id: number, data?: { name?: string; scopes?: string[]; expires_in_days?: number }) => api<{ token: string; info: AgentToken }>("POST", `/me/agent-tokens/${id}/rotate`, data || {}),
  session: (data?: { scopes?: string[]; actor?: string }) => api<AgentSession>("POST", "/me/agent-session", data || {}),
  audit: (params?: Record<string, string>) => api<PaginatedData<AgentAuditLog>>("GET", "/me/agent-audit", params),
}

export const notificationsApi = {
  list: (params?: { unread?: boolean; page?: number; pageSize?: number }) => api<NotificationPage>("GET", "/notifications", params),
  markRead: (id: number) => api<null>("POST", `/notifications/${id}/read`),
  markAllRead: () => api<null>("POST", "/notifications/read-all"),
}

export const adminNotificationsApi = {
  publish: (data: { user_id?: number; type: string; title: string; content: string; link?: string }) => api<{ sent: number; type: string }>("POST", "/admin/notifications", data),
}

export const suggestionsApi = {
  list: (status = "pending") => api<Suggestion[]>("GET", "/suggestions", { status }),
  resolve: (id: number, data: { accept: boolean; dish_ids?: number[]; meal_type?: string; meal_date?: string }) =>
    api<{ created: MealRecord[] }>("POST", `/suggestions/${id}/resolve`, data),
}

export const assistantApi = {
  status: () => api<AssistantStatus>("GET", "/assistant/status"),
  sessions: () => api<ChatSession[]>("GET", "/assistant/sessions"),
  messages: (id: number) =>
    api<{ session: ChatSession; messages: { id: number; role: "user" | "assistant"; content: string; cards: ChatCard[]; created_at: string }[] }>(
      "GET", `/assistant/sessions/${id}`,
    ),
  deleteSession: (id: number) => api<null>("DELETE", `/assistant/sessions/${id}`),
}

export type ChatEvent =
  | { event: "session"; data: { session_id: number; title: string } }
  | { event: "delta"; data: { text: string } }
  | { event: "tool_start"; data: { id: string; name: string; label: string } }
  | { event: "tool_end"; data: { id: string; name: string; ok: boolean; error?: string; card?: ChatCard | null } }
  | { event: "error"; data: { message: string } }
  | { event: "done"; data: { session_id?: number; message_id?: number } }

// streamChat POST /api/assistant/chat，逐个回调 SSE 事件（fetch + ReadableStream，EventSource 不支持 POST）
export async function streamChat(
  body: { session_id?: number; message: string },
  onEvent: (e: ChatEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const token = getToken()
  const res = await fetch("/api/assistant/chat", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
    signal,
  })
  if (res.status === 401) {
    window.dispatchEvent(new Event("auth-expired"))
    throw new ApiError("登录已过期", 40100, 401)
  }
  if (!res.ok || !res.body) {
    let message = "AI 助手暂时不可用"
    try {
      const j = await res.json()
      if (j?.message) message = j.message
    } catch {
      /* ignore */
    }
    throw new ApiError(message, -1, res.status)
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buffer.indexOf("\n\n")) >= 0) {
      const chunk = buffer.slice(0, idx)
      buffer = buffer.slice(idx + 2)
      let event = "message"
      const dataLines: string[] = []
      for (const line of chunk.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim()
        else if (line.startsWith("data:")) dataLines.push(line.slice(5).trimStart())
      }
      if (dataLines.length === 0) continue
      try {
        onEvent({ event, data: JSON.parse(dataLines.join("\n")) } as ChatEvent)
      } catch {
        /* 忽略坏帧 */
      }
    }
  }
}

export type { ChatMessage }

export const dishesApi = {
  list: (params?: Record<string, string>) => api<PaginatedData<Dish>>("GET", "/dishes", params),
  categoryCounts: () => api<DishCategoryCounts & { mine: number }>("GET", "/dishes/category-counts"),
  get: (id: number) => api<Dish>("GET", `/dishes/${id}`),
  records: (id: number) => api<DishRecordsResponse>("GET", `/dishes/${id}/records`),
  create: (data: DishInput) => api<Dish>("POST", "/dishes", data),
  update: (id: number, data: DishInput) => api<Dish>("PUT", `/dishes/${id}`, data),
  delete: (id: number) => api<null>("DELETE", `/dishes/${id}`),
  toggle: (id: number) => api<Dish>("PUT", `/dishes/${id}/toggle`),
  clone: (id: number, toPublic = false) => api<Dish>("POST", `/dishes/${id}/clone${toPublic ? "?public=1" : ""}`),
  batchToggle: (ids: number[], enabled: boolean) => api<null>("POST", "/dishes/batch-toggle", { ids, enabled }),
  batchDelete: (ids: number[]) => api<null>("POST", "/dishes/batch-delete", { ids }),
  batchCategory: (ids: number[], category: string) => api<null>("POST", "/dishes/batch-category", { ids, category }),
}

export const pickApi = {
  lunch: (count?: number) => api<PickResult>("POST", `/pick/lunch${count ? `?count=${count}` : ""}`),
  dinner: (count?: number) => api<PickResult>("POST", `/pick/dinner${count ? `?count=${count}` : ""}`),
  mood: (mood: string) => api<PickResult>("POST", "/pick/mood", { mood }),
  tomorrow: (data: { meal_type: string; profile: string; count: number; exclude_ids?: number[] }) =>
    api<PickResult>("POST", "/pick/tomorrow", data),
  blindBox: () => api<BlindBoxResult>("POST", "/pick/blind-box"),
  // 智能推荐：口味画像 + 约束打分，返回带理由的结果
  smart: (data: SmartPickRequest) => api<SmartPickResult>("POST", "/pick/smart", data),
}

export const profileApi = {
  get: (days?: number) => api<TasteProfile>("GET", "/profile", days ? { days: String(days) } : undefined),
}

// 行为埋点：浏览 / 采纳 / 拒绝推荐等，失败静默（不影响主流程）
export const behaviorApi = {
  log: (data: BehaviorEventInput) => api<null>("POST", "/behavior", data).catch(() => null),
}

export const recordsApi = {
  list: (params?: Record<string, string>) => api<PaginatedData<MealRecord>>("GET", "/records", params),
  create: (data: Partial<MealRecord>) => api<MealRecord>("POST", "/records", data),
  batchCreate: (data: Partial<MealRecord>[]) =>
    api<{ created: MealRecord[]; skipped: number; total: number }>("POST", "/records/batch", { records: data }),
  update: (id: number, data: Partial<MealRecord>) => api<MealRecord>("PUT", `/records/${id}`, data),
  delete: (id: number) => api<null>("DELETE", `/records/${id}`),
}

export const favoritesApi = {
  list: () => api<Dish[]>("GET", "/favorites"),
  overview: () => api<FavoriteOverview>("GET", "/favorites/overview"),
  add: (dishId: number) => api<null>("POST", `/favorites/${dishId}`),
  remove: (dishId: number) => api<null>("DELETE", `/favorites/${dishId}`),
}

export const uploadApi = {
  image: (file: File) => {
    const formData = new FormData()
    formData.append("image", file)
    return defaultClient.post<{ code: number; message: string; data: { url: string; filename: string } }>("/upload/image", formData, {
      headers: { "Content-Type": "multipart/form-data" },
      timeout: 60000,
    })
  },
  remove: (url: string) => api<null>("DELETE", "/upload/image", { url }),
}

export const statsApi = {
  get: () => api<UserStats>("GET", "/stats"),
}

export const quotesApi = {
  list: () => api<Quote[]>("GET", "/quotes"),
  create: (data: { content: string; scene: string }) => api<Quote>("POST", "/quotes", data),
  update: (id: number, data: { content: string; scene: string }) => api<Quote>("PUT", `/quotes/${id}`, data),
  delete: (id: number) => api<null>("DELETE", `/quotes/${id}`),
}

export const achievementsApi = {
  list: async () => {
    const data = await api<Achievement[] | null>("GET", "/achievements")
    return Array.isArray(data) ? data : []
  },
  unlock: (id: number) => api<null>("POST", `/achievements/${id}/unlock`),
  toggle: (id: number) => api<null>("POST", `/achievements/${id}/toggle`),
  create: (data: Partial<Achievement>) => api<Achievement>("POST", "/achievements", data),
  update: (id: number, data: Partial<Achievement>) => api<Achievement>("PUT", `/achievements/${id}`, data),
  delete: (id: number) => api<null>("DELETE", `/achievements/${id}`),
}

export const weekPlanApi = {
  get: () => api<WeekPlan>("GET", "/week-plan"),
  regenerate: () => api<WeekPlan>("POST", "/week-plan/regenerate"),
}

export const shoppingListApi = {
  get: () => api<ShoppingCategory[]>("GET", "/shopping-list"),
  toggle: (data: { item_name: string; meal_date: string; checked: boolean }) => api<null>("POST", "/shopping-list/toggle", data),
  setInventory: (data: { item_name: string; in_stock: boolean }) => api<null>("POST", "/shopping-list/inventory", data),
}

export const shoppingCategoriesApi = {
  list: () => api<ShoppingCategoryOverride[]>("GET", "/shopping-categories"),
  save: (data: { item_name: string; category: string }) => api<null>("POST", "/shopping-categories", data),
  delete: (itemName: string) => api<null>("DELETE", `/shopping-categories/${encodeURIComponent(itemName)}`),
}

export const holidaysApi = {
  upcoming: () => api<Holiday[]>("GET", "/holidays/upcoming"),
}

export const dayRatingApi = {
  get: (date: string) => api<DayRating>("GET", `/day-rating?date=${date}`),
  list: (params: { date_from: string; date_to: string }) => api<DayRating[]>("GET", "/day-ratings", params),
  updateHomeMood: (data: { meal_date: string; home_mood: string }) => api<DayRating>("POST", "/day-rating/home-mood", data),
  create: (data: { meal_date: string; mood?: string; remark?: string; photos?: string }) => api<DayRating>("POST", "/day-rating", data),
}

export const photoWallApi = {
  get: () => api<PhotoWall>("GET", "/photo-wall"),
}

// 个人设置（语音、盲盒、去重天数、每日道数）
export const settingsApi = {
  get: () => api<Record<string, unknown>>("GET", "/settings"),
  update: (settings: Record<string, string>) => api<null>("PUT", "/settings", { settings }),
}

// ---- 管理员 ----

export const adminApi = {
  dashboard: () => api<DashboardData>("GET", "/admin/dashboard"),
  users: (params?: Record<string, string>) => api<PaginatedData<AdminUser>>("GET", "/admin/users", params),
  updateUser: (id: number, data: { role?: string; disabled?: boolean }) => api<User>("PUT", `/admin/users/${id}`, data),
  settings: () => api<SiteSettings>("GET", "/admin/settings"),
  updateSettings: (settings: Record<string, string>) => api<null>("PUT", "/admin/settings", { settings }),
  testLLM: () => api<null>("POST", "/admin/llm/test"),
}

// 兼容旧代码
export const dashboardApi = { get: adminApi.dashboard }
