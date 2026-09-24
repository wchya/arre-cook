import { useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeft, ArrowUp, BookOpen, Clock3, History, MessageCircle, Plus, Trash2, X } from "lucide-react"
import toast from "react-hot-toast"
import { assistantApi, errorMessage, streamChat, type ChatEvent } from "@/api"
import { isImageUrl } from "@/lib/dish-image"
import type { ChatCard, ChatMessage, ChatToolStep } from "@/types"

const fallbackSuggestions = ["今晚吃什么？", "我最近的饮食报告", "推荐一道省事的晚餐"]

function MessageCards({ cards }: { cards: ChatCard[] }) {
  return <div className="mt-3 space-y-3">
    {cards.map((card, index) => card.type === "action" ?
      <div key={index} className="border-l-2 border-mint bg-mint-light/50 px-3 py-2 text-sm text-text2">{card.text}</div> :
      <div key={index} className="border-y border-border py-2">
        {card.title && <div className="mb-1 text-xs font-semibold text-text3">{card.title}</div>}
        <div className="divide-y divide-border">
          {card.items?.map(({ dish, reasons }) => <Link key={dish.id} to={`/dishes/${dish.id}`} className="flex min-w-0 items-center gap-3 py-2 text-left">
            {isImageUrl(dish.image) ? <img src={dish.image} alt="" className="h-12 w-12 shrink-0 rounded-md object-cover" /> : <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-md bg-primary-light text-xl">🍽</span>}
            <span className="min-w-0 flex-1"><span className="block truncate text-sm font-semibold text-text">{dish.name}</span><span className="mt-0.5 block truncate text-xs text-text3">{reasons?.join(" · ") || [dish.category, dish.cook_time > 0 ? `${dish.cook_time} 分钟` : ""].filter(Boolean).join(" · ")}</span></span>
          </Link>)}
        </div>
      </div>) }
  </div>
}

function ChatBubble({ message }: { message: ChatMessage }) {
  const mine = message.role === "user"
  return <div className={`flex ${mine ? "justify-end" : "justify-start"}`}>
    <div className={`min-w-0 max-w-[90%] rounded-lg px-3.5 py-3 text-sm leading-relaxed ${mine ? "bg-primary text-white" : "border border-border bg-card text-text"}`}>
      {message.content ? <div className="whitespace-pre-wrap break-words">{message.content}</div> : message.streaming ? <div className="text-text3">正在整理...</div> : null}
      {message.tools && message.tools.length > 0 && <div className="mt-2 space-y-1 border-t border-border pt-2 text-xs text-text3">
        {message.tools.map((tool) => <div key={tool.id} className="flex items-center gap-1.5"><span className={tool.status === "error" ? "text-red" : tool.status === "ok" ? "text-mint" : "text-primary"}>{tool.status === "running" ? "···" : tool.status === "ok" ? "✓" : "!"}</span><span>{tool.label}</span>{tool.error && <span className="truncate text-red">{tool.error}</span>}</div>)}
      </div>}
      {message.cards.length > 0 && <MessageCards cards={message.cards} />}
      {message.error && <p className="mt-2 text-xs text-red">{message.error}</p>}
    </div>
  </div>
}

export default function AssistantChat() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [sessionId, setSessionId] = useState<number | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draft, setDraft] = useState("")
  const [busy, setBusy] = useState(false)
  const [loadingSession, setLoadingSession] = useState(false)
  const [showHistory, setShowHistory] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)
  const controllerRef = useRef<AbortController | null>(null)
  const requestRef = useRef(0)
  const { data: status } = useQuery({ queryKey: ["assistant-status"], queryFn: assistantApi.status })
  const { data: sessions = [] } = useQuery({ queryKey: ["assistant-sessions"], queryFn: assistantApi.sessions })

  useEffect(() => { scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" }) }, [messages])
  useEffect(() => () => { controllerRef.current?.abort(); requestRef.current++ }, [])

  function stopCurrent() {
    controllerRef.current?.abort()
    controllerRef.current = null
    requestRef.current++
    setBusy(false)
  }

  function newChat() {
    stopCurrent()
    setSessionId(null)
    setMessages([])
    setDraft("")
    setLoadingSession(false)
    setShowHistory(false)
  }

  async function openSession(id: number) {
    stopCurrent()
    const request = requestRef.current
    setSessionId(id)
    setMessages([])
    setLoadingSession(true)
    setShowHistory(false)
    try {
      const result = await assistantApi.messages(id)
      if (request === requestRef.current) setMessages(result.messages)
    } catch (err) {
      if (request === requestRef.current) toast.error(errorMessage(err))
    } finally {
      if (request === requestRef.current) setLoadingSession(false)
    }
  }

  async function deleteSession(id: number) {
    if (!window.confirm("删除这段对话？")) return
    try {
      await assistantApi.deleteSession(id)
      if (sessionId === id) newChat()
      await qc.invalidateQueries({ queryKey: ["assistant-sessions"] })
    } catch (err) { toast.error(errorMessage(err)) }
  }

  function updateAssistant(id: string, update: (message: ChatMessage) => ChatMessage) {
    setMessages((current) => current.map((message) => message.id === id ? update(message) : message))
  }

  async function send(value = draft) {
    const text = value.trim()
    if (!text || busy || loadingSession) return
    setDraft("")
    setBusy(true)
    const assistantId = `assistant-${Date.now()}`
    const controller = new AbortController()
    controllerRef.current = controller
    const request = ++requestRef.current
    let completed = false
    setMessages((current) => [...current,
      { id: `user-${Date.now()}`, role: "user", content: text, cards: [] },
      { id: assistantId, role: "assistant", content: "", cards: [], streaming: true },
    ])

    try {
      await streamChat({ session_id: sessionId ?? undefined, message: text }, (event: ChatEvent) => {
        if (request !== requestRef.current) return
        if (event.event === "session") {
          setSessionId(event.data.session_id)
          void qc.invalidateQueries({ queryKey: ["assistant-sessions"] })
        } else if (event.event === "delta") {
          updateAssistant(assistantId, (message) => ({ ...message, content: message.content + event.data.text }))
        } else if (event.event === "tool_start") {
          const step: ChatToolStep = { id: event.data.id, name: event.data.name, label: event.data.label, status: "running" }
          updateAssistant(assistantId, (message) => ({ ...message, tools: [...(message.tools || []), step] }))
        } else if (event.event === "tool_end") {
          updateAssistant(assistantId, (message) => ({ ...message,
            tools: message.tools?.map((step) => step.id === event.data.id ? { ...step, status: event.data.ok ? "ok" : "error", error: event.data.error } : step),
            cards: event.data.card ? [...message.cards, event.data.card] : message.cards,
          }))
        } else if (event.event === "error") {
          updateAssistant(assistantId, (message) => ({ ...message, error: event.data.message }))
        } else if (event.event === "done") {
          completed = true
          updateAssistant(assistantId, (message) => ({ ...message, id: event.data.message_id || message.id, streaming: false }))
        }
      }, controller.signal)
      if (!completed) throw new Error("连接中断，请查看对话记录后重试")
      await qc.invalidateQueries()
    } catch (err) {
      if (!controller.signal.aborted && request === requestRef.current) {
        updateAssistant(assistantId, (message) => ({ ...message, streaming: false, error: errorMessage(err) }))
      }
    } finally {
      if (request === requestRef.current) {
        controllerRef.current = null
        setBusy(false)
        void qc.invalidateQueries({ queryKey: ["assistant-sessions"] })
      }
    }
  }

  return <div className="flex h-dvh min-h-0 flex-col bg-bg text-text">
    <header className="shrink-0 border-b border-border bg-card px-4 pt-[env(safe-area-inset-top)]">
      <div className="mx-auto flex h-14 max-w-[640px] items-center gap-3">
        <button onClick={() => navigate("/")} aria-label="返回首页" title="返回首页" className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-text2"><ArrowLeft size={21} /></button>
        <div className="min-w-0 flex-1"><h1 className="truncate text-base font-bold">AI 饮食助手</h1><p className="truncate text-xs text-text3">{status?.llm_enabled ? "可以聊饮食、菜谱与记录" : "基础推荐与饮食报告"}</p></div>
        <button onClick={() => setShowHistory(true)} title="对话记录" aria-label="对话记录" className="flex h-9 w-9 items-center justify-center rounded-md text-text2"><History size={20} /></button>
        <button onClick={newChat} title="新对话" aria-label="新对话" className="flex h-9 w-9 items-center justify-center rounded-md text-text2"><Plus size={21} /></button>
      </div>
    </header>

    <main ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto overscroll-y-contain">
      <div className="mx-auto max-w-[640px] space-y-4 px-4 py-5">
        {!status?.llm_enabled && status && <div className="border-l-2 border-primary bg-primary-light/40 px-3 py-2 text-xs leading-relaxed text-text2">当前未连接 AI 模型，可获取基础菜谱推荐和已记录的饮食报告。对话写入暂不可用，请使用饮食记录或菜谱页面填写。</div>}
        {messages.length === 0 && !loadingSession && <div className="pt-5">
          <div className="mb-5 flex items-center gap-3"><div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary-light text-primary"><MessageCircle size={22} /></div><div><h2 className="text-lg font-bold">今天想吃什么？</h2><p className="text-xs text-text3">从菜谱、实际记录和你的口味出发</p></div></div>
          <div className="space-y-2">{(status?.suggestions || fallbackSuggestions).map((suggestion) => <button key={suggestion} onClick={() => void send(suggestion)} className="flex w-full items-center justify-between rounded-md border border-border bg-card px-3 py-3 text-left text-sm text-text2"><span className="min-w-0 break-words">{suggestion}</span><ArrowUp size={15} className="ml-3 shrink-0 rotate-45 text-text3" /></button>)}</div>
          <div className="mt-5 flex gap-4 text-xs font-medium text-primary"><button onClick={() => navigate("/health")} className="flex items-center gap-1"><Clock3 size={15} />饮食记录</button><button onClick={() => navigate("/assistant")} className="flex items-center gap-1"><BookOpen size={15} />选菜工具</button></div>
        </div>}
        {loadingSession && <p className="py-8 text-center text-sm text-text3">正在读取对话...</p>}
        {messages.map((message) => <ChatBubble key={message.id} message={message} />)}
      </div>
    </main>

    <form onSubmit={(event) => { event.preventDefault(); void send() }} className="shrink-0 border-t border-border bg-card px-4 pb-[calc(10px+env(safe-area-inset-bottom))] pt-3">
      <div className="mx-auto flex max-w-[640px] items-end gap-2 rounded-lg border border-border bg-bg p-1.5">
        <textarea value={draft} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); void send() } }} rows={1} maxLength={1000} placeholder="聊聊今天吃什么" aria-label="发送给 AI 助手的消息" className="max-h-28 min-h-10 min-w-0 flex-1 resize-none bg-transparent px-2 py-2 text-sm text-text outline-none placeholder:text-text3" />
        <button disabled={!draft.trim() || busy || loadingSession} title="发送" aria-label="发送" className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-primary text-white disabled:opacity-40"><ArrowUp size={19} /></button>
      </div>
    </form>

    {showHistory && <div className="fixed inset-0 z-[150] flex justify-end bg-black/40" role="presentation" onClick={() => setShowHistory(false)}>
      <aside role="dialog" aria-modal="true" aria-label="对话记录" onClick={(event) => event.stopPropagation()} className="flex h-full w-[min(86vw,360px)] flex-col bg-card pt-[env(safe-area-inset-top)] shadow-xl">
        <div className="flex h-14 shrink-0 items-center justify-between border-b border-border px-4"><h2 className="font-bold">对话记录</h2><button onClick={() => setShowHistory(false)} title="关闭" aria-label="关闭" className="flex h-9 w-9 items-center justify-center"><X size={20} /></button></div>
        <div className="min-h-0 flex-1 overflow-y-auto">{sessions.length === 0 ? <p className="p-5 text-sm text-text3">还没有对话记录</p> : sessions.map((session) => <div key={session.id} className={`flex items-center gap-1 border-b border-border px-3 ${sessionId === session.id ? "bg-primary-light/50" : ""}`}><button onClick={() => void openSession(session.id)} className="min-w-0 flex-1 py-3 text-left"><span className="block truncate text-sm font-medium">{session.title}</span><span className="mt-1 block text-xs text-text3">{new Date(session.updated_at).toLocaleDateString("zh-CN")}</span></button><button onClick={() => void deleteSession(session.id)} title="删除对话" aria-label={`删除${session.title}`} className="flex h-9 w-9 shrink-0 items-center justify-center text-text3"><Trash2 size={16} /></button></div>)}</div>
        <button onClick={newChat} className="m-4 flex h-11 shrink-0 items-center justify-center gap-2 rounded-md bg-primary text-sm font-semibold text-white"><Plus size={18} />新对话</button>
      </aside>
    </div>}
  </div>
}
