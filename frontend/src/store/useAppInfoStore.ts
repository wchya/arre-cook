import { create } from "zustand"
import { appInfoApi } from "@/api"

interface AppInfoState {
  appName: string
  agentEmbedUrl: string
  announcement: string
  aiEnabled: boolean
  wechatLogin: boolean
  loaded: boolean
  fetch: () => Promise<void>
  setAppName: (name: string) => void
  setAgentEmbedUrl: (url: string) => void
}

const APP_NAME_KEY = "ninimenu_app_name"

function getStoredName(): string {
  try {
    return localStorage.getItem(APP_NAME_KEY) || "NiniMenu"
  } catch {
    return "NiniMenu"
  }
}

export const useAppInfoStore = create<AppInfoState>((set) => ({
  appName: getStoredName(),
  agentEmbedUrl: "",
  announcement: "",
  aiEnabled: false,
  wechatLogin: false,
  loaded: false,
  fetch: async () => {
    try {
      const data = await appInfoApi.get()
      const name = (data.app_name || "").trim() || "NiniMenu"
      try {
        localStorage.setItem(APP_NAME_KEY, name)
      } catch {
        /* ignore */
      }
      document.title = name
      set({
        appName: name,
        agentEmbedUrl: (data.agent_embed_url || "").trim(),
        announcement: (data.announcement || "").trim(),
        aiEnabled: !!data.ai_enabled,
        wechatLogin: !!data.wechat_login,
        loaded: true,
      })
    } catch {
      set({ loaded: true })
    }
  },
  setAppName: (name: string) => {
    try {
      localStorage.setItem(APP_NAME_KEY, name)
    } catch {
      /* ignore */
    }
    set({ appName: name })
    document.title = name
  },
  setAgentEmbedUrl: (url: string) => set({ agentEmbedUrl: url.trim() }),
}))
