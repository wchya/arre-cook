// A page may own more than one sheet. Restore its menu only after the last closes.
const owners = new WeakMap()
function currentPage() { const pages = getCurrentPages(); return pages[pages.length - 1] }
function hasVisibleSheet(page) { const set = page && owners.get(page); return Boolean(set && set.size) }
function lock(component) {
  const page = component._sheetOwner || currentPage()
  if (!page) return
  component._sheetOwner = page
  const set = owners.get(page) || new Set()
  set.add(component)
  owners.set(page, set)
  const tab = typeof page.getTabBar === "function" && page.getTabBar()
  if (tab) tab.setData({ hidden: true })
}
function unlock(component) {
  const page = component._sheetOwner
  const set = page && owners.get(page)
  if (!set) return
  set.delete(component)
  if (set.size || currentPage() !== page) return
  const tab = typeof page.getTabBar === "function" && page.getTabBar()
  if (tab) tab.setData({ hidden: false })
}
function releasePage(page) { if (page) owners.delete(page) }
module.exports = { lock, unlock, hasVisibleSheet, releasePage }
