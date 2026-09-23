import { create } from "zustand"
import axios from "axios"

interface AppInfoState {
  appName: string
  agentEmbedUrl: string
  loaded: boolean
  fetch: () => Promise<void>
  setAppName: (name: string) => void
  setAgentEmbedUrl: (url: string) => void
}

const APP_NAME_KEY = "ninimenu_app_name"
const AGENT_EMBED_KEY = "ninimenu_agent_embed_url"

function getStoredName(): string {
  return localStorage.getItem(APP_NAME_KEY) || "NiniMenu"
}

function getStoredEmbedUrl(): string {
  return localStorage.getItem(AGENT_EMBED_KEY) || ""
}

export const useAppInfoStore = create<AppInfoState>((set) => ({
  appName: getStoredName(),
  agentEmbedUrl: getStoredEmbedUrl(),
  loaded: false,
  fetch: async () => {
    try {
      const res = await axios.get("/api/app-info")
      if (res.data?.code === 0 && res.data?.data) {
        const data = res.data.data as { app_name?: string; agent_embed_url?: string }
        const patch: Partial<AppInfoState> = { loaded: true }
        if (data.app_name) {
          localStorage.setItem(APP_NAME_KEY, data.app_name)
          patch.appName = data.app_name
          document.title = data.app_name
        }
        const embed = (data.agent_embed_url || "").trim()
        localStorage.setItem(AGENT_EMBED_KEY, embed)
        patch.agentEmbedUrl = embed
        set(patch)
      } else {
        set({ loaded: true })
      }
    } catch {
      set({ loaded: true })
    }
  },
  setAppName: (name: string) => {
    localStorage.setItem(APP_NAME_KEY, name)
    set({ appName: name })
    document.title = name
  },
  setAgentEmbedUrl: (url: string) => {
    const trimmed = url.trim()
    localStorage.setItem(AGENT_EMBED_KEY, trimmed)
    set({ agentEmbedUrl: trimmed })
  },
}))
