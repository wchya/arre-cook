const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const { createRequire } = require('node:module')
const front = createRequire(path.resolve(__dirname, '../../frontend/package.json'))
const ts = front('typescript'), React = front('react')
function component(file, stubs) {
 const mod = { exports: {} }
 const code = ts.transpileModule(fs.readFileSync(path.resolve(__dirname, '../../frontend/src', file), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 } }).outputText
 new Function('require', 'module', 'exports', code)(name => stubs[name] || front(name), mod, mod.exports)
 return mod.exports.default
}

test('Web report confirms omission and actual breakfast, refreshes conflicts, blocks old session writes', async () => {
 const { JSDOM } = front('jsdom')
 const dom = new JSDOM('<div id="root"></div>', { url: 'https://example.test' })
 const previous = { window: global.window, document: global.document, IS_REACT_ACT_ENVIRONMENT: global.IS_REACT_ACT_ENVIRONMENT }
 global.window = dom.window; global.document = dom.window.document; global.IS_REACT_ACT_ENVIRONMENT = true
 const { createRoot } = front('react-dom/client')
 let epoch = 1, fail = false
 const calls = [], invalidations = []
 class ApiError extends Error { constructor() { super('changed'); this.status = 409 } }
 const Panel = component('components/HealthReportPanel.tsx', {
  'react-router-dom': { useNavigate: () => () => {} },
  '@tanstack/react-query': { useQueryClient: () => ({ invalidateQueries: async value => invalidations.push(value.queryKey[0]) }) },
  'react-hot-toast': { success() {}, error() {} },
  '@/api/client': { getSessionEpoch: () => epoch, isCurrentSession: value => value === epoch },
  '@/lib/health-date': { healthDate: () => '2026-09-28' },
  '@/api': { ApiError, errorMessage: e => e.message, healthApi: { setMealStatus: async (...args) => { calls.push(['status', ...args]); if (fail) throw new ApiError() } }, recordsApi: { create: async data => calls.push(['record', data]) } },
 })
 const report = { from: '2026-09-22', to: '2026-09-28', logged_days: 0, complete_days: 0, item_count: 0, meal_event_count: 0, days: [{ date: '2026-09-28', fingerprint: 'snapshot', status: 'partial', item_count: 0, evidence: [], meals: [{ meal_type: 'breakfast', status: 'unknown' }] }], insights: [], food_group_days: {}, plan_actions: [], recommendations: [], method_notes: [], plans: [{ id: 4, meal_date: '2026-09-28', meal_type: 'breakfast', dish_id: 8, dish_name: '鸡蛋', available: true, status: 'planned' }] }
 const root = createRoot(document.querySelector('#root'))
 const button = text => [...document.querySelectorAll('button')].find(b => b.textContent === text)
 const click = text => React.act(async () => button(text).click())
 try {
  window.confirm = () => false
  await React.act(async () => root.render(React.createElement(Panel, { report, onRecord() {} })))
  await click('标记这餐未吃'); assert.equal(calls.length, 0)
  window.confirm = () => true
  await click('标记这餐未吃')
  assert.deepEqual(calls[0], ['status', '2026-09-28', 'breakfast', 'snapshot', true])
  assert.ok(invalidations.includes('health-report'))
  assert.equal(button('确认当天已记完整'), undefined, 'omission-only day cannot confirm complete')
  fail = true; invalidations.length = 0
  await click('标记这餐未吃'); assert.deepEqual(invalidations, ['health-report'])
  fail = false
  await click('记为吃过')
  assert.deepEqual(calls[2], ['record', { dish_id: 8, meal_type: 'breakfast', meal_date: '2026-09-28' }])
  report.days[0].meals[0].status = 'not_eaten'
  await React.act(async () => root.render(React.createElement(Panel, { report, onRecord() {} })))
  await click('撤销未吃标记'); assert.equal(calls[3][4], false)
  epoch++
  await click('撤销未吃标记'); await click('记为吃过'); assert.equal(calls.length, 4)
 } finally {
  await React.act(async () => root.unmount()); dom.window.close()
  for (const key of Object.keys(previous)) { if (previous[key] === undefined) delete global[key]; else global[key] = previous[key] }
 }
})

test('Web homepage summary isolates queries and shows loading/error/actual counts with report link', () => {
 const { renderToStaticMarkup } = front('react-dom/server')
 let result = { isPending: true }, query
 const Summary = component('components/HealthSummaryCard.tsx', {
  '@tanstack/react-query': { useQuery: options => { query = options; return { refetch() {}, ...result } } },
  'react-router-dom': { Link: ({ to, children, ...props }) => React.createElement('a', { href: to, ...props }, children) },
  '@/api': { healthApi: {} }, '@/store/useAuthStore': { useAuthStore: select => select({ user: { id: 23 } }) },
  '@/lib/health-date': { healthDate: () => '2026-09-28' },
 })
 const render = () => renderToStaticMarkup(React.createElement(Summary))
 assert.match(render(), /正在整理/); assert.deepEqual(query.queryKey, ['health-report', 'summary', 23, '2026-09-28'])
 result = { error: new Error('offline') }; assert.match(render(), /重试/)
 result = { data: { from: '2026-09-22', to: '2026-09-28', logged_days: 0, complete_days: 0, meal_event_count: 0, item_count: 0, not_eaten_meals: 1, headline: '从今天吃过的一餐开始记录' } }
 const html = render()
 assert.match(html, /href="\/health\?view=report"/); assert.match(html, /0 天有实际饮食记录/); assert.match(html, /1 餐明确未吃/); assert.match(html, /未记录不代表没有吃/)
})
