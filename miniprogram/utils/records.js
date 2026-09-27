const api = require("./api")

// A calendar month can contain more than the API's 100-row page limit.
async function forDates(from, to) {
  const params = { date_from: from, date_to: to, pageSize: 100 }
  const first = await api.get("/records", params)
  const items = (first.items || []).slice()
  for (let page = 2; items.length < first.total; page++) {
    const next = await api.get("/records", { ...params, page })
    if (!next.items || !next.items.length) break
    items.push(...next.items)
  }
  return { ...first, items: Array.from(new Map(items.map((item) => [item.id, item])).values()) }
}

module.exports = { forDates }
