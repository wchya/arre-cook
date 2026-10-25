export function recommendationDate(date = new Date()) {
  const beijing = new Date(date.getTime() + 8 * 60 * 60 * 1000)
  return beijing.toISOString().slice(0, 10)
}
