export interface DishIngredient {
  name: string
  amount?: string
}

export interface DishStep {
  text: string
  time?: number
  image?: string
}

export interface Dish {
  id: number
  name: string
  image_url: string
  // 后端返回真正的 JSON 数组；历史数据可能是字符串数组或对象数组，统一用 asArray 消费
  images: string[]
  video_url: string
  category: string
  meal_type: string
  taste: string
  ingredients: Array<string | DishIngredient>
  seasonings: Array<string | DishIngredient>
  steps: Array<string | DishStep>
  cook_time: number
  difficulty: string
  remark: string
  favorite: boolean
  enabled: boolean
  // 0 = 公共菜谱；否则为该用户的私房菜
  owner_id: number
  family_id: number
  tags: string[]
  sort_order: number
  created_at: string
  updated_at: string
}

// 创建/更新菜品的请求体：JSON 数组字段以字符串形式提交，与后端 CreateDishRequest 对应
export interface DishInput {
  name: string
  image_url?: string
  images?: string
  video_url?: string
  category?: string
  meal_type?: string
  taste?: string
  ingredients?: string
  seasonings?: string
  steps?: string
  cook_time?: number
  difficulty?: string
  remark?: string
  tags?: string
  sort_order?: number
  public?: boolean
  family?: boolean
}

export interface FamilyMember {
  user_id: number
  nickname: string
  email: string
  avatar: string
  role: "owner" | "member"
  joined_at: string
}

export interface FamilyInvitation {
  id: number
  email: string
  expires_at: string
}

export interface FamilySnapshot {
  family: { id: number; name: string; owner_id: number } | null
  role: "owner" | "member" | ""
  members: FamilyMember[]
  invitations: FamilyInvitation[]
}

export interface FamilyPlanItem {
  id: number
  meal_date: string
  meal_type: string
  dish: Dish
  added_by: number
}

export interface FamilyShoppingItem {
  id: number
  name: string
  amount: string
  checked: boolean
  added_by: number
}

export interface FoodJournalEntry {
  id: number
  meal_date: string
  meal_type: string
  dish_name: string
  cuisine: string
  food_groups: string[]
  notes: string
  created_at: string
}

export type FoodJournalInput = Omit<FoodJournalEntry, "id" | "created_at">

export interface HealthReport {
  period_days: number
  from: string
  to: string
  logged_days: number
  meal_count: number
  cuisine_counts: { name: string; count: number }[]
  food_group_days: Record<string, number>
  days: { date: string; meal_count: number; cuisines: string[]; food_groups: string[] }[]
  insights: string[]
  plan_actions: string[]
  recommendations: { dish: Dish; reason: string }[]
}

export interface MealRecord {
  id: number
  dish_id: number
  dish_name: string
  dish_image_url: string
  dish_emoji: string
  meal_type: string
  meal_date: string
  rating: number
  remark: string
  mood: string
  photo: string
  created_at: string
}

export interface PickResult {
  dishes: Dish[]
  quote: string
}

export interface BlindBoxResult {
  hint: string
  dish: Dish
  quote: string
}

export interface PaginatedData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

export interface StatsData {
  total_records: number
  total_dishes: number
  lunch_count: number
  dinner_count: number
  top_dishes: { dish_name: string; count: number }[]
}

export interface DashboardData {
  total_dishes: number
  enabled_dishes: number
  disabled_dishes: number
  private_dishes: number
  total_records: number
  today_records: number
  favorite_count: number
  user_count: number
  active_users_7d: number
  new_users_7d: number
  agent_tokens: number
  chat_messages_7d: number
  pending_suggestions: number
  ai_enabled: boolean
  smtp_enabled: boolean
  category_counts: { category: string; count: number }[]
  top_dishes: { dish_id: number; dish_name: string; count: number }[]
  recent_records: { id: number; dish_id: number; dish_name: string; meal_type: string; meal_date: string; mood: string; rating: number; user_name: string }[]
  week_trend: { date: string; count: number }[]
  difficulty_counts: { difficulty: string; count: number }[]
}

export interface Achievement {
  id: number
  code: string
  name: string
  description: string
  icon: string
  condition: string
  is_unlocked: boolean
  unlocked_at: string | null
  created_at: string
}

export interface WeekDayPlan {
  date: string
  day_name: string
  lunch: Dish[]
  dinner: Dish[]
}

export interface WeekPlan {
  days: WeekDayPlan[]
}

export interface ShoppingCategory {
  category: string
  items: ShoppingItem[]
}

export interface ShoppingItem {
  name: string
  amount: string
  checked: boolean
  in_stock: boolean
}

export interface ShoppingCategoryOverride {
  item_name: string
  category: string
}

export interface Holiday {
  id: number
  name: string
  date: string
  dish_ids: string
  greeting: string
  created_at: string
}

export interface Quote {
  id: number
  content: string
  scene: string
  enabled: boolean
  created_at: string
}

export interface DayRating {
  id: number
  meal_date: string
  home_mood: string
  mood: string
  remark: string
  photos: string
  created_at: string
  updated_at: string
}

export interface PhotoWallRecord {
  dish_id: number
  dish_name: string
  meal_type: string
  mood: string
  remark: string
}

export interface PhotoWallDay {
  meal_date: string
  photos: string[]
  home_mood: string
  day_mood: string
  day_remark: string
  records: PhotoWallRecord[]
  photo_count: number
}

export interface PhotoWall {
  days: PhotoWallDay[]
  total_days: number
  total_photos: number
}

export interface DishRecordWithDay {
  id: number
  meal_type: string
  meal_date: string
  rating: number
  remark: string
  mood: string
  photo: string
  home_mood: string
  day_mood: string
  day_remark: string
  photos: string[]
}

export interface CategoryCount {
  category: string
  count: number
}

export interface DishCategoryCounts {
  total: number
  categories: CategoryCount[]
}

export interface FavoriteOverviewItem {
  dish: Dish
  favorite_created_at: string
  record_count: number
  lunch_count: number
  dinner_count: number
  last_eaten_at: string
  avg_rating: number
  is_never_cooked: boolean
}

export interface FavoriteCategorySummary {
  category: string
  count: number
  avg_cook_time: number
  cooked_count: number
  never_cooked_count: number
  quick_dish_id: number
  quick_dish_name: string
}

export interface FavoriteOverviewStats {
  total: number
  category_total: number
  avg_cook_time: number
  cooked_count: number
  never_cooked_count: number
  quick_dish: Dish | null
  most_cooked_dish: Dish | null
}

export interface FavoriteOverview {
  items: FavoriteOverviewItem[]
  categories: FavoriteCategorySummary[]
  stats: FavoriteOverviewStats
  quick_picks: FavoriteOverviewItem[]
  most_cooked: FavoriteOverviewItem[]
  need_try: FavoriteOverviewItem[]
}

export interface DishRecordsStats {
  total_count: number
  lunch_count: number
  dinner_count: number
  yum_percent: number
  ok_percent: number
  no_percent: number
  avg_rating: number
  last_date: string
  avg_interval: number
}

export interface DishRecordsResponse {
  records: DishRecordWithDay[]
  stats: DishRecordsStats
}

// ---- 智能推荐 / 口味画像 / 行为事件 ----

export interface RecommendItem {
  dish: Dish
  score: number
  reasons: string[]
}

export interface SmartPickRequest {
  meal_type?: string
  mood?: string
  count?: number
  keyword?: string
  tastes?: string[]
  categories?: string[]
  max_cook_time?: number
  difficulty?: string
  include_ingredients?: string[]
  exclude_ingredients?: string[]
  exclude_dish_ids?: number[]
  exclude_recent_days?: number
  mode?: string
}

export interface SmartPickResult {
  items: RecommendItem[]
  profile_summary: string
  applied: Record<string, unknown>
  quote: string
}

export interface WeightItem {
  name: string
  weight: number
  count: number
}

export interface DishBrief {
  id: number
  name: string
  category: string
  taste: string
  cook_time: number
  difficulty: string
}

export interface DishFrequency {
  dish_id: number
  dish_name: string
  count: number
  last_date: string
}

export interface TasteProfile {
  generated_at: string
  window_days: number
  repeat_days: number
  total_records: number
  window_records: number
  distinct_dishes: number
  meal_type_counts: Record<string, number>
  taste_weights: WeightItem[]
  category_weights: WeightItem[]
  difficulty_counts: Record<string, number>
  avg_cook_time: number
  spicy_ratio: number
  top_ingredients: WeightItem[]
  top_dishes: DishFrequency[]
  recent_dish_ids: number[]
  favorite_dishes: DishBrief[]
  liked_dishes: DishBrief[]
  disliked_dishes: DishBrief[]
  mood_counts: Record<string, number>
  home_mood_counts: Record<string, number>
  behavior_counts: Record<string, number>
  most_viewed_dishes: DishFrequency[]
  summary: string
}

export interface BehaviorEventInput {
  event_type: "view" | "recommend" | "accept" | "reject" | "search" | "chat" | "feedback" | "custom"
  dish_id?: number
  dish_name?: string
  source?: string
  actor?: string
  meta?: Record<string, unknown>
}

export interface AppInfo {
  app_name: string
  agent_embed_url: string
  announcement: string
  ai_enabled: boolean
  wechat_login: boolean
  register_open: boolean
}

// ---- 账号 ----

export interface User {
  id: number
  email: string
  username: string
  nickname: string
  avatar: string
  role: "user" | "admin"
  wechat_bound: boolean
  has_password: boolean
  disabled: boolean
  last_login_at: string | null
  created_at: string
}

export interface LoginResult {
  token: string
  expires_at: string
  user: User
  created?: boolean
  need_bind?: boolean
}

export interface AuthOptions {
  email: boolean
  email_dev: boolean
  email_domains: string[] | null
  wechat: boolean
  register_open: boolean
}

export interface Preferences {
  avoid_ingredients: string[]
  allergies: string[]
  favorite_tastes: string[]
  spice_level: number
  household_size: number
  max_cook_time: number
  goals: string
  notes: string
  updated_at?: string
}

export interface UserStats {
  total_records: number
  total_dishes: number
  private_dishes: number
  lunch_count: number
  dinner_count: number
  favorite_count: number
  distinct_dishes: number
  cook_days: number
  current_streak: number
  this_month: number
  top_dishes: { dish_id: number; dish_name: string; count: number }[]
  category_counts: { name: string; count: number }[]
  week_trend: { date: string; count: number }[]
}

// ---- AI 连接 ----

export interface AgentToken {
  id: number
  name: string
  prefix: string
  scopes: string[]
  active: boolean
  last_used_at: string | null
  expires_at: string | null
  revoked_at: string | null
  created_at: string
}

export interface AgentTokenList {
  tokens: AgentToken[]
  scopes: Record<string, string>
  presets: Record<string, string[]>
  mcp_url: string
  api_url: string
}

export interface AgentAuditLog {
  id: number
  token_id: number
  actor: string
  channel: string
  tool: string
  args: string
  status: "ok" | "error" | "denied"
  error: string
  duration_ms: number
  created_at: string
}

export interface AgentSession {
  token: string
  expires_at: string
  scopes: string[]
  api_base: string
  mcp_url: string
}

export interface Notification {
  id: number
  type: "system_update" | "feature" | "maintenance" | "health_tip" | "agent_security" | "family" | string
  title: string
  content: string
  link: string
  read_at: string | null
  created_at: string
}

export interface NotificationPage {
  items: Notification[]
  total: number
  unread: number
  page: number
  page_size: number
}

export interface Suggestion {
  id: number
  source: string
  title: string
  reason: string
  meal_type: "" | "lunch" | "dinner"
  meal_date: string
  status: "pending" | "accepted" | "dismissed" | "expired"
  expires_at: string | null
  resolved_at: string | null
  created_at: string
  dishes: Dish[]
}

// ---- AI 助手 ----

export interface DishCardData {
  id: number
  name: string
  category: string
  taste: string
  cook_time: number
  difficulty: string
  meal_type: string
  ingredients: string[]
  image?: string
  favorite: boolean
  private?: boolean
}

export interface ChatCard {
  type: "dishes" | "action"
  title?: string
  text?: string
  items?: { dish: DishCardData; reasons?: string[]; score?: number }[]
}

export interface ChatToolStep {
  id: string
  name: string
  label: string
  status: "running" | "ok" | "error"
  error?: string
}

export interface ChatMessage {
  id: number | string
  role: "user" | "assistant"
  content: string
  cards: ChatCard[]
  tools?: ChatToolStep[]
  streaming?: boolean
  error?: string
  created_at?: string
}

export interface ChatSession {
  id: number
  title: string
  created_at: string
  updated_at: string
}

export interface AssistantStatus {
  llm_enabled: boolean
  model: string
  suggestions: string[]
}

export interface AdminUser extends User {
  record_count: number
}

export interface SiteSettings {
  [key: string]: unknown
  llm_effective?: { base_url: string; model: string; enabled: boolean; from_env: boolean }
  llm_api_key_set?: boolean
  smtp_enabled?: boolean
  wechat_enabled?: boolean
}
