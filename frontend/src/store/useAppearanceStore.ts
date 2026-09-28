import { create } from "zustand"
import { APPEARANCE_KEY, applyAppearance, parseAppearance, readAppearance, type Appearance } from "@/lib/appearance"

type AppearanceState = Appearance & {
  systemDark: boolean
  storageAvailable: boolean
  change: (patch: Partial<Appearance>) => void
}

export const useAppearanceStore = create<AppearanceState>((set, get) => ({
  ...readAppearance(),
  systemDark: window.matchMedia("(prefers-color-scheme: dark)").matches,
  storageAvailable: true,
  change: (patch) => {
    const { palette, mode, systemDark } = { ...get(), ...patch }
    const appearance = { palette, mode }
    let storageAvailable = true
    try { localStorage.setItem(APPEARANCE_KEY, JSON.stringify(appearance)) }
    catch { storageAvailable = false }
    applyAppearance(appearance, systemDark)
    set({ ...appearance, storageAvailable })
  },
}))

export function initializeAppearance() {
  const media = window.matchMedia("(prefers-color-scheme: dark)")
  const sync = () => {
    useAppearanceStore.setState({ systemDark: media.matches })
    applyAppearance(useAppearanceStore.getState(), media.matches)
  }
  const onStorage = (event: StorageEvent) => {
    if (event.key !== APPEARANCE_KEY && event.key !== null) return
    if (event.storageArea && event.storageArea !== window.localStorage) return
    const appearance = parseAppearance(event.newValue)
    applyAppearance(appearance, media.matches)
    useAppearanceStore.setState({ ...appearance, systemDark: media.matches, storageAvailable: true })
  }
  sync()
  media.addEventListener("change", sync)
  window.addEventListener("storage", onStorage)
  return () => {
    media.removeEventListener("change", sync)
    window.removeEventListener("storage", onStorage)
  }
}
