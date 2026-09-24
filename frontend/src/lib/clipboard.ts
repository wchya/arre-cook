import toast from "react-hot-toast"

// 复制到剪贴板：优先 Clipboard API，不可用时（如部分 WebView）回退到 execCommand
export async function copyText(text: string, okMsg = "已复制") {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement("textarea")
      ta.value = text
      ta.setAttribute("readonly", "")
      ta.style.position = "fixed"
      ta.style.opacity = "0"
      document.body.appendChild(ta)
      ta.select()
      document.execCommand("copy")
      document.body.removeChild(ta)
    }
    toast.success(okMsg)
  } catch {
    toast.error("复制失败，请长按手动复制")
  }
}
