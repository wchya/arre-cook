// A share title is not a URL. Only open one validated platform link, including
// when an older saved recipe still contains the complete share message.
export function videoLink(raw: unknown): { url: string; name: string; key: string } | null {
  if (typeof raw !== "string" || raw.length > 6144) return null
  const links = raw.match(/https?:\/\/[^\s<>"'，。；！）】]+/g)
  if (links?.length !== 1) return null
  try {
    const u = new URL(links[0].replace(/[.,;!)\]}]+$/, ""))
    if (u.username || u.password || u.port || u.pathname.includes("%") || u.pathname.includes("\\")) return null
    const host = u.hostname.toLowerCase()
    if (host === "b23.tv" && /^\/[a-zA-Z0-9_-]{1,80}\/?$/.test(u.pathname)) return { url: `https://b23.tv${u.pathname}`, key: "bilibili", name: "哔哩哔哩" }
    if (["bilibili.com", "www.bilibili.com", "m.bilibili.com"].includes(host) && /^\/video\/(BV[0-9A-Za-z]{10}|av[0-9]{1,20})\/?$/.test(u.pathname)) {
      const pages = u.searchParams.getAll("p")
      const page = pages.length ? Number(pages[0]) : 1
      if (pages.length > 1 || !Number.isInteger(page) || page < 1 || page > 100) return null
      return { url: `https://www.bilibili.com${u.pathname.replace(/\/$/, "")}/${page > 1 ? `?p=${page}` : ""}`, key: "bilibili", name: "哔哩哔哩" }
    }
    if (host === "v.douyin.com" && /^\/[a-zA-Z0-9_-]{1,80}\/?$/.test(u.pathname)) return { url: `https://v.douyin.com${u.pathname}`, key: "douyin", name: "抖音" }
    if (["douyin.com", "www.douyin.com", "iesdouyin.com", "www.iesdouyin.com"].includes(host)) {
      const id = u.pathname.match(/^\/(?:share\/)?video\/([0-9]{1,20})\/?$/)?.[1] || (u.pathname === "/" && u.searchParams.getAll("modal_id").length === 1 ? u.searchParams.get("modal_id") : "")
      if (id && /^[0-9]{1,20}$/.test(id)) return { url: `https://www.douyin.com/video/${id}`, key: "douyin", name: "抖音" }
    }
  } catch { /* Malformed historical links must not become navigation targets. */ }
  return null
}
