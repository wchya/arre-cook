import { useState } from "react"
import { Check, Monitor, Moon, Palette, Sun, X } from "lucide-react"
import AnimatedBottomSheet from "@/components/AnimatedBottomSheet"
import { displayModes, palettes } from "@/lib/appearance"
import { useAppearanceStore } from "@/store/useAppearanceStore"

const modeIcons = { light: Sun, dark: Moon, system: Monitor }

export default function ThemePicker({ variant = "compact" }: { variant?: "compact" | "row" }) {
  const [open, setOpen] = useState(false)
  const { palette, mode, change, storageAvailable } = useAppearanceStore()
  const current = palettes.find((item) => item.id === palette)!
  const currentMode = displayModes.find((item) => item.id === mode)!

  return <>
    <button type="button" onClick={() => setOpen(true)} aria-haspopup="dialog" aria-expanded={open}
      aria-label={`切换主题，当前${current.name}，${currentMode.name}`}
      className={variant === "row" ? "theme-trigger theme-trigger--row" : "theme-trigger"}>
      <span className="theme-trigger__icon"><Palette size={19} strokeWidth={2} /></span>
      {variant === "row" ? <span className="min-w-0 flex-1 text-left"><span className="block text-sm font-semibold">页面主题</span><span className="block text-xs text-text3">{current.name} · {currentMode.name}</span></span> : <span className="theme-trigger__label">换肤</span>}
      {variant === "row" && <span className="text-text3" aria-hidden="true">›</span>}
    </button>
    {open && <AnimatedBottomSheet label="页面主题" onClose={() => setOpen(false)} className="theme-sheet">
      {({ close }) => <>
        <div className="theme-sheet__header">
          <div><h2>换个颜色，好好吃饭</h2><p>选一套喜欢的配色，整站即刻切换</p></div>
          <button type="button" onClick={close} aria-label="关闭主题设置" className="theme-close"><X size={20} /></button>
        </div>
        <fieldset><legend className="theme-legend">页面配色</legend>
          <div className="theme-options">
            {palettes.map((item) => <label key={item.id} className={`theme-option ${palette === item.id ? "is-selected" : ""}`}>
              <input type="radio" name="page-palette" value={item.id} checked={palette === item.id} onChange={() => change({ palette: item.id })} />
              <span data-palette={item.id} className="theme-preview" aria-hidden="true">
                <span className="theme-preview__card"><span className="theme-preview__line" /><span className="theme-preview__ticket" /><span className="theme-preview__button" /></span>
              </span>
              <span className="theme-option__name">{item.name}<span className="theme-option__check" aria-hidden="true">{palette === item.id && <Check size={13} strokeWidth={3} />}</span></span>
              <span className="sr-only">{item.description}</span>
            </label>)}
          </div>
        </fieldset>
        <fieldset className="mt-6"><legend className="theme-legend">显示模式</legend>
          <div className="theme-modes">
            {displayModes.map((item) => {
              const Icon = modeIcons[item.id]
              return <label key={item.id} className={`theme-mode ${mode === item.id ? "is-selected" : ""}`}>
                <input type="radio" name="display-mode" value={item.id} checked={mode === item.id} onChange={() => change({ mode: item.id })} />
                <Icon size={18} /><span>{item.name}</span>
              </label>
            })}
          </div>
        </fieldset>
        <p className="theme-storage" role="status">{storageAvailable ? "偏好自动保存在此设备，下次打开依然是你喜欢的样子。" : "当前浏览器无法保存偏好，本次切换已生效。"}</p>
        <button type="button" onClick={close} className="theme-done">就用这个主题</button>
      </>}
    </AnimatedBottomSheet>}
  </>
}
