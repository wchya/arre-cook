import { useEffect, useRef, type RefObject } from "react"

const dialogStack: HTMLElement[] = []
let rootWasInert = false
const focusableSelector = 'button:not([disabled]), a[href], input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'

// Shared by portal dialogs. Only the top dialog owns Escape and the focus loop.
export function useDialog(panelRef: RefObject<HTMLElement | null>, onClose: () => void) {
  const closeRef = useRef(onClose)
  // Capture the trigger before React mounts any auto-focused dialog input.
  const previousFocusRef = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null)
  useEffect(() => { closeRef.current = onClose }, [onClose])
  useEffect(() => {
    const panel = panelRef.current
    if (!panel) return
    const previousFocus = previousFocusRef.current
    const appRoot = document.getElementById("root")
    if (dialogStack.length === 0) rootWasInert = appRoot?.inert ?? false
    if (appRoot) appRoot.inert = true
    dialogStack.push(panel)
    const frame = requestAnimationFrame(() => {
      if (panel.contains(document.activeElement)) return
      const first = panel.querySelector<HTMLElement>('[autofocus], [data-autofocus], input:not([disabled]):not([type="hidden"]), textarea:not([disabled])')
      ;(first || panel).focus({ preventScroll: true })
    })
    function onKeyDown(event: KeyboardEvent) {
      if (dialogStack.at(-1) !== panel) return
      if (event.key === "Escape" && !event.isComposing) {
        event.preventDefault()
        event.stopPropagation()
        closeRef.current()
      }
      if (event.key !== "Tab") return
      const controls = Array.from(panel!.querySelectorAll<HTMLElement>(focusableSelector)).filter((el) => el.getClientRects().length && !el.closest('[hidden], [inert]'))
      const first = controls[0], last = controls.at(-1)
      if (!first) { event.preventDefault(); panel!.focus(); return }
      if (event.shiftKey && (document.activeElement === first || document.activeElement === panel)) {
        event.preventDefault(); last?.focus()
      } else if (!event.shiftKey && (document.activeElement === last || !panel!.contains(document.activeElement))) {
        event.preventDefault(); first.focus()
      }
    }
    document.addEventListener("keydown", onKeyDown, true)
    return () => {
      cancelAnimationFrame(frame)
      document.removeEventListener("keydown", onKeyDown, true)
      const index = dialogStack.indexOf(panel)
      if (index >= 0) dialogStack.splice(index, 1)
      if (appRoot) appRoot.inert = dialogStack.length > 0 || rootWasInert
      if (previousFocus?.isConnected && !previousFocus.closest("[inert]")) previousFocus.focus({ preventScroll: true })
    }
  }, [panelRef])
}
