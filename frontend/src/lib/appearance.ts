export const APPEARANCE_KEY = "ninimenu_appearance"

export const palettes = [
  { id: "lime", name: "青柠", description: "清新青柠，轻盈每一餐" },
  { id: "peach", name: "暖桃", description: "熟悉的暖色，家的味道" },
  { id: "ocean", name: "海盐蓝", description: "清透海盐，慢慢享用" },
  { id: "berry", name: "莓果", description: "柔和莓果，一点小甜蜜" },
] as const

export const displayModes = [
  { id: "light", name: "浅色" },
  { id: "dark", name: "深色" },
  { id: "system", name: "跟随系统" },
] as const

export type Palette = typeof palettes[number]["id"]
export type DisplayMode = typeof displayModes[number]["id"]
export type Appearance = { palette: Palette; mode: DisplayMode }

export function parseAppearance(raw: string | null): Appearance {
  try {
    const value = JSON.parse(raw || "null")
    return {
      palette: palettes.some((item) => item.id === value?.palette) ? value.palette : "lime",
      mode: displayModes.some((item) => item.id === value?.mode) ? value.mode : "system",
    }
  } catch {
    return { palette: "lime", mode: "system" }
  }
}

export function readAppearance(): Appearance {
  try { return parseAppearance(localStorage.getItem(APPEARANCE_KEY)) }
  catch { return parseAppearance(null) }
}

export function applyAppearance(appearance: Appearance, systemDark: boolean) {
  const root = document.documentElement
  root.dataset.palette = appearance.palette
  root.dataset.mode = appearance.mode === "system" ? (systemDark ? "dark" : "light") : appearance.mode
  // Read the resolved palette so browser chrome follows manual choices as well.
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", getComputedStyle(root).getPropertyValue("--color-bg").trim())
}
