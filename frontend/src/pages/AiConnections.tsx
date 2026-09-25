import { useMemo, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { agentConnApi, errorMessage } from "@/api"
import PageHeader from "@/components/PageHeader"
import AnimatedBottomSheet from "@/components/AnimatedBottomSheet"
import { copyText } from "@/lib/clipboard"
import type { AgentToken } from "@/types"
import toast from "react-hot-toast"
import { Check, Copy, KeyRound, Plug, Plus, RefreshCw, ShieldCheck, Trash2 } from "lucide-react"

const PRESETS = [
  { key: "readonly", title: "只读", desc: "读取口味画像、菜谱、记录，适合做分析报告" },
  { key: "advisor", title: "顾问（推荐）", desc: "只读 + 向你推送建议 + 记录反馈，由你决定是否采纳" },
  { key: "full", title: "完全代理", desc: "还可以替你记一餐、收藏、改偏好、重排菜单" },
] as const

const EXPIRES = [
  { days: 0, label: "永久" },
  { days: 30, label: "30 天" },
  { days: 90, label: "90 天" },
  { days: 365, label: "1 年" },
]

const NAME_SUGGESTIONS = ["DeepSeek", "Hermes", "Claude", "Cursor", "Dify", "Coze"]

type SnippetKey = "mcp" | "hermes" | "deepseek" | "curl" | "openapi"

function fmtTime(s: string | null) {
  if (!s) return "从未"
  const d = new Date(s)
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60) return "刚刚"
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

export default function AiConnections() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ["agent-tokens"], queryFn: () => agentConnApi.list() })
  const { data: audit } = useQuery({ queryKey: ["agent-audit"], queryFn: () => agentConnApi.audit({ pageSize: "20" }) })

  const [creating, setCreating] = useState(false)
  const [name, setName] = useState("")
  const [preset, setPreset] = useState<string>("advisor")
  const [expires, setExpires] = useState(0)
  const [created, setCreated] = useState<{ token: string; info: AgentToken } | null>(null)
  // 最近一次新建的令牌：关闭弹窗后仍用于填充下方的接入示例（离开页面即丢弃，不落盘）
  const [lastToken, setLastToken] = useState("")
  const [snippet, setSnippet] = useState<SnippetKey>("mcp")

  const createMut = useMutation({
    mutationFn: () => agentConnApi.create({ name: name.trim(), scopes: [preset], expires_in_days: expires }),
    onSuccess: (res) => {
      setCreated(res)
      setLastToken(res.token)
      setCreating(false)
      setName("")
      qc.invalidateQueries({ queryKey: ["agent-tokens"] })
    },
    onError: (err) => toast.error(errorMessage(err, "创建失败")),
  })

  const revokeMut = useMutation({
    mutationFn: (id: number) => agentConnApi.revoke(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agent-tokens"] })
      toast.success("已撤销，该智能体立即失去访问权限")
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const rotateMut = useMutation({
    mutationFn: (id: number) => agentConnApi.rotate(id),
    onSuccess: (res) => {
      setCreated(res)
      setLastToken(res.token)
      qc.invalidateQueries({ queryKey: ["agent-tokens"] })
      toast.success("已轮换，旧令牌立即失效")
    },
    onError: (err) => toast.error(errorMessage(err, "轮换失败")),
  })

  const mcpURL = data?.mcp_url || `${location.origin}/mcp`
  const apiURL = data?.api_url || `${location.origin}/api/agent`
  const token = lastToken || "nm_你的令牌"

  const snippets = useMemo<Record<SnippetKey, { label: string; code: string; note: string }>>(() => ({
    mcp: {
      label: "MCP 通用",
      note: "Claude Desktop / Cursor / Cherry Studio 等支持远程 MCP（Streamable HTTP）的客户端",
      code: JSON.stringify({ mcpServers: { ninimenu: { type: "http", url: mcpURL, headers: { Authorization: `Bearer ${token}` } } } }, null, 2),
    },
    hermes: {
      label: "Hermes",
      note: "Hermes Agent：写入 ~/.hermes/config.yaml 的 mcp_servers（参考配置，以所用版本文档为准）",
      code: `mcp_servers:\n  ninimenu:\n    url: "${mcpURL}"\n    headers:\n      Authorization: "Bearer ${token}"`,
    },
    deepseek: {
      label: "DeepSeek 函数调用",
      note: "拉取工具清单交给 DeepSeek（OpenAI 兼容 tools 格式），模型要调用哪个工具就 POST 到对应地址",
      code: `import requests
from openai import OpenAI

API = "${apiURL}"
H = {"Authorization": "Bearer ${token}"}
tools = requests.get(f"{API}/tools", headers=H).json()["data"]

client = OpenAI(api_key="sk-你的DeepSeekKey", base_url="https://api.deepseek.com")
messages = [{"role": "user", "content": "今晚吃什么？想吃辣的"}]
while True:
    msg = client.chat.completions.create(model="deepseek-chat", messages=messages, tools=tools).choices[0].message
    messages.append(msg)
    if not msg.tool_calls:
        print(msg.content); break
    for call in msg.tool_calls:
        r = requests.post(f"{API}/tools/{call.function.name}", headers=H, data=call.function.arguments or "{}")
        messages.append({"role": "tool", "tool_call_id": call.id, "content": r.text})`,
    },
    curl: {
      label: "curl",
      note: "直接调用工具（请求体即工具参数）",
      code: `curl -s -X POST "${apiURL}/tools/recommend_dishes" \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{"meal_type":"dinner","mood":"spicy","count":3}'`,
    },
    openapi: {
      label: "OpenAPI",
      note: "Dify / Coze / GPTs Actions 导入此地址，鉴权选 Bearer 并填入令牌",
      code: `${apiURL}/openapi.json`,
    },
  }), [mcpURL, apiURL, token])

  const tokens = data?.tokens || []
  const summary = data?.summary
  const active = tokens.filter((t) => t.active)
  const inactive = tokens.filter((t) => !t.active)
  const scopeLabels = data?.scopes || {}

  return (
    <div className="animate-fadeUp pb-10">
      <PageHeader title="AI 连接" subtitle="把你的食谱数据安全地交给智能体" icon={Plug} onBack={() => navigate(-1)} />
      <div className="mx-auto max-w-[640px] space-y-4 px-5 py-4">
        <div className="rounded-[22px] bg-gradient-to-br from-purple-light via-card to-primary-light p-4">
          <div className="mb-2 flex items-center gap-2 text-[15px] font-extrabold">
            <ShieldCheck size={18} className="text-purple" /> 只属于你的数据通道
          </div>
          <ul className="space-y-1 text-[12px] leading-relaxed text-text2">
            <li>• 每个令牌只能访问<b>你本人</b>的数据，并且只能做你勾选的事。</li>
            <li>• 支持 MCP、DeepSeek 函数调用、OpenAPI 三种接入方式。</li>
            <li>• 智能体的每次调用都会记在下方的「调用记录」里，可随时撤销。</li>
          </ul>
        </div>

        <div className="flex items-center justify-between">
          <div className="text-[15px] font-extrabold">已连接的智能体</div>
          <button onClick={() => setCreating(true)} className="flex h-9 items-center gap-1 rounded-full bg-primary px-4 text-[13px] font-bold text-white active:scale-95">
            <Plus size={15} strokeWidth={2.6} /> 新建令牌
          </button>
        </div>

        {summary && (
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {[
              ["有效令牌", summary.active_tokens, "个"],
              ["24 小时调用", summary.calls_last_24_hours, "次"],
              ["累计调用", summary.calls, "次"],
              ["成功率", summary.calls ? `${Math.round(summary.success_calls * 100 / summary.calls)}%` : "暂无", ""],
            ].map(([label, value, suffix]) => (
              <div key={String(label)} className="rounded-2xl border border-border bg-card px-3 py-3">
                <div className="text-[11px] text-text3">{label}</div>
                <div className="mt-1 text-[20px] font-extrabold">{value}<span className="ml-0.5 text-[11px] font-semibold text-text3">{suffix}</span></div>
              </div>
            ))}
          </div>
        )}

        {isLoading ? (
          <div className="skeleton h-24 rounded-[22px]" />
        ) : active.length === 0 ? (
          <div className="rounded-[22px] border border-dashed border-border2 bg-card px-4 py-8 text-center">
            <KeyRound size={28} className="mx-auto mb-2 text-text4" />
            <div className="text-[14px] font-bold text-text2">还没有连接任何智能体</div>
            <div className="mt-1 text-[12px] text-text3">新建一个令牌，填进 Hermes、DeepSeek 或其他 MCP 客户端即可</div>
          </div>
        ) : (
          <div className="space-y-2.5">
            {active.map((t) => (
              <div key={t.id} className="rounded-[20px] border border-border bg-card p-3.5 shadow-[0_1px_3px_rgba(0,0,0,.03)]">
                <div className="flex items-start gap-3">
                  <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-purple-light text-[15px] font-black text-purple">
                    {t.name.slice(0, 1).toUpperCase()}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-[15px] font-extrabold">{t.name}</span>
                      <span className="rounded-md bg-bg px-1.5 py-px font-mono text-[10px] text-text3">{t.prefix}…</span>
                    </div>
                    <div className="mt-0.5 text-[11px] text-text3">
                      最近使用 {fmtTime(t.last_used_at)} · {t.expires_at ? `${new Date(t.expires_at).toLocaleDateString()} 过期` : "永久有效"}
                    </div>
                    <div className="mt-2 flex flex-wrap gap-1">
                      {t.scopes.map((s) => (
                        <span key={s} className="rounded-full bg-bg px-2 py-0.5 text-[10px] font-semibold text-text2">{scopeLabels[s] || s}</span>
                      ))}
                    </div>
                  </div>
                  <button
                    onClick={() => confirm(`撤销「${t.name}」？撤销后它将立即无法访问你的数据。`) && revokeMut.mutate(t.id)}
                    className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-red-light text-red active:scale-90"
                    aria-label="撤销"
                  >
                    <Trash2 size={14} />
                  </button>
                  <button
                    onClick={() => confirm(`轮换「${t.name}」？旧令牌会立即失效。`) && rotateMut.mutate(t.id)}
                    className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-yellow-light text-yellow-dark active:scale-90"
                    aria-label="轮换令牌"
                    title="轮换令牌"
                  >
                    <RefreshCw size={14} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        <section className="rounded-[22px] border border-border bg-card p-4">
          <div className="mb-1 text-[15px] font-extrabold">接入方式</div>
          <div className="mb-3 text-[11px] text-text3">把下面的配置里的令牌换成你新建的令牌</div>
          <div className="mb-2 flex gap-1.5 overflow-x-auto scrollbar-none">
            {(Object.keys(snippets) as SnippetKey[]).map((k) => (
              <button
                key={k}
                onClick={() => setSnippet(k)}
                className={`shrink-0 rounded-full px-3 py-1.5 text-[12px] font-bold ${snippet === k ? "bg-text text-bg" : "bg-bg text-text2"}`}
              >
                {snippets[k].label}
              </button>
            ))}
          </div>
          <div className="mb-2 text-[11px] leading-relaxed text-text3">{snippets[snippet].note}</div>
          <div className="relative">
            <pre className="max-h-72 overflow-auto rounded-2xl bg-[#1d1d22] p-3.5 pr-11 text-[11px] leading-relaxed text-[#e8e8ee]"><code>{snippets[snippet].code}</code></pre>
            <button onClick={() => copyText(snippets[snippet].code)} className="absolute right-2 top-2 flex h-8 w-8 items-center justify-center rounded-xl bg-white/10 text-white active:scale-90" aria-label="复制">
              <Copy size={14} />
            </button>
          </div>
        </section>

        <section className="rounded-[22px] border border-border bg-card p-4">
          <div className="mb-3 text-[15px] font-extrabold">调用记录</div>
          {!audit || audit.items.length === 0 ? (
            <div className="py-4 text-center text-[12px] text-text3">暂无智能体调用</div>
          ) : (
            <div className="divide-y divide-border">
              {audit.items.map((a) => (
                <div key={a.id} className="flex items-center gap-2.5 py-2">
                  <span className={`h-2 w-2 shrink-0 rounded-full ${a.status === "ok" ? "bg-mint" : a.status === "denied" ? "bg-yellow" : "bg-red"}`} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[13px] font-semibold">
                      {a.actor} <span className="font-mono text-[12px] text-text2">· {a.tool}</span>
                    </div>
                    {a.error && <div className="truncate text-[11px] text-red">{a.error}</div>}
                  </div>
                  <span className="shrink-0 text-[11px] text-text3">{a.channel} · {fmtTime(a.created_at)}</span>
                </div>
              ))}
            </div>
          )}
        </section>

        {inactive.length > 0 && (
          <div className="text-center text-[11px] text-text4">另有 {inactive.length} 个已撤销或过期的令牌</div>
        )}
      </div>

      {creating && (
        <AnimatedBottomSheet onClose={() => setCreating(false)} className="app-scroll max-h-[88dvh] overflow-y-auto rounded-t-3xl p-5 pb-8">
          {({ close }) => (
            <>
              <div className="mx-auto mb-4 h-1 w-10 rounded-full bg-border2" />
              <div className="mb-4 text-[18px] font-extrabold">新建智能体令牌</div>
              <div className="mb-1.5 text-[12px] font-bold text-text2">给它起个名字</div>
              <input
                value={name}
                maxLength={32}
                onChange={(e) => setName(e.target.value)}
                placeholder="例如 Hermes、DeepSeek"
                className="mb-2 h-12 w-full rounded-2xl border-[1.5px] border-border bg-bg px-4 text-[15px] outline-none focus:border-primary"
              />
              <div className="mb-4 flex flex-wrap gap-1.5">
                {NAME_SUGGESTIONS.map((n) => (
                  <button key={n} onClick={() => setName(n)} className="rounded-full bg-bg px-2.5 py-1 text-[11px] font-semibold text-text2 active:scale-95">{n}</button>
                ))}
              </div>
              <div className="mb-1.5 text-[12px] font-bold text-text2">允许它做什么</div>
              <div className="mb-4 space-y-2">
                {PRESETS.map((p) => (
                  <button
                    key={p.key}
                    onClick={() => setPreset(p.key)}
                    className={`flex w-full items-start gap-3 rounded-2xl border-[1.5px] p-3 text-left transition-all ${preset === p.key ? "border-primary bg-primary-light" : "border-border bg-bg"}`}
                  >
                    <span className={`mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border-2 ${preset === p.key ? "border-primary bg-primary text-white" : "border-border2"}`}>
                      {preset === p.key && <Check size={12} strokeWidth={3} />}
                    </span>
                    <span>
                      <span className="block text-[14px] font-bold">{p.title}</span>
                      <span className="block text-[11px] leading-relaxed text-text2">{p.desc}</span>
                    </span>
                  </button>
                ))}
              </div>
              <div className="mb-1.5 text-[12px] font-bold text-text2">有效期</div>
              <div className="mb-5 grid grid-cols-4 gap-2">
                {EXPIRES.map((e) => (
                  <button key={e.days} onClick={() => setExpires(e.days)} className={`h-10 rounded-2xl border-[1.5px] text-[12px] font-bold ${expires === e.days ? "border-mint bg-mint-light text-mint" : "border-border bg-bg text-text2"}`}>
                    {e.label}
                  </button>
                ))}
              </div>
              <div className="flex gap-3">
                <button onClick={close} className="h-12 flex-1 rounded-2xl border-[1.5px] border-border text-sm font-semibold">取消</button>
                <button onClick={() => createMut.mutate()} disabled={!name.trim() || createMut.isPending} className="h-12 flex-[2] rounded-2xl bg-primary text-sm font-extrabold text-white disabled:opacity-50">
                  {createMut.isPending ? "创建中…" : "创建令牌"}
                </button>
              </div>
            </>
          )}
        </AnimatedBottomSheet>
      )}

      {created && (
        <AnimatedBottomSheet onClose={() => setCreated(null)} className="rounded-t-3xl p-5 pb-8">
          {({ close }) => (
            <>
              <div className="mx-auto mb-4 h-1 w-10 rounded-full bg-border2" />
              <div className="mb-1 text-[18px] font-extrabold">令牌已创建 🎉</div>
              <div className="mb-3 text-[12px] leading-relaxed text-red">这是唯一一次显示完整令牌，请立即复制保存。遗失后只能撤销重建。</div>
              <button onClick={() => copyText(created.token, "令牌已复制")} className="mb-4 flex w-full items-center gap-2 rounded-2xl bg-bg p-3.5 text-left active:scale-[.99]">
                <code className="min-w-0 flex-1 break-all font-mono text-[12px] text-text">{created.token}</code>
                <Copy size={16} className="shrink-0 text-primary" />
              </button>
              <div className="mb-4 text-[12px] leading-relaxed text-text2">
                MCP 地址：<code className="break-all font-mono text-[11px]">{mcpURL}</code>
                <br />下方「接入方式」里的示例已自动填好这个令牌，可以直接复制。
              </div>
              <button onClick={() => { close(); setSnippet("mcp") }} className="h-12 w-full rounded-2xl bg-primary text-sm font-extrabold text-white">我已保存</button>
            </>
          )}
        </AnimatedBottomSheet>
      )}
    </div>
  )
}
