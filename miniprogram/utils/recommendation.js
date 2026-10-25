const api = require("./api")

function feedback(recommendationId, dishId, eventType) {
  if (!recommendationId || !dishId) return Promise.resolve()
  return api.post("/recommendation-feedback", {
    recommendation_id: recommendationId,
    dish_id: dishId,
    event_type: eventType,
    // Reversible preferences use the server's state-aware idempotency key.
    ...(eventType === "dislike" || eventType === "undo_dislike" ? {} : {
      request_key: `${recommendationId}:${dishId}:${eventType}`,
    }),
  })
}

// Candidates fetched in advance are not impressions. Native viewport observation
// records only cards actually displayed to the user.
function observe(page, selector, recommendationId, dishId) {
  if (!recommendationId || !dishId || !page.createIntersectionObserver) return
  page._impressions = page._impressions || new Set()
  const key = `${recommendationId}:${dishId}`
  if (page._impressions.has(key)) return
  const observer = page.createIntersectionObserver({ thresholds: [0.5] })
  page._recommendationObservers = page._recommendationObservers || []
  page._recommendationObservers.push(observer)
  observer.relativeToViewport().observe(selector, (entry) => {
    if (entry.intersectionRatio < 0.5 || page._recommendationHidden) return
    page._impressions.add(key)
    feedback(recommendationId, dishId, "impression").catch(() => page._impressions.delete(key))
    observer.disconnect()
  })
}

function disconnect(page) {
  ;(page._recommendationObservers || []).forEach((observer) => observer.disconnect())
  page._recommendationObservers = []
}

function dateKey(date = new Date()) {
  return new Date(date.getTime() + 8 * 60 * 60 * 1000).toISOString().slice(0, 10)
}

module.exports = { feedback, observe, disconnect, dateKey }
