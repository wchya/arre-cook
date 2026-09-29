export function healthDate(offset = 0) {
  return new Date(Date.now() + (8 * 60 * 60 + offset * 86400) * 1000).toISOString().slice(0, 10)
}
