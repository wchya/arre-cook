const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const root = path.resolve(__dirname, '../..')
const front = createRequire(path.join(root, 'frontend/package.json'))
const dish = id => ({ id, name: `菜${id}`, ingredients: [], seasonings: [], tags: [] })
const result = (id, ids) => ({ recommendation_id: id, items: ids.map(id => ({ dish: dish(id), reasons: ['适合这一餐'], score: 76 })) })

function mini(name) {
  let definition
  const calls = [], toasts = [], observers = []
  const api = {
    token: () => 'alice',
    get: async () => [],
    post: async (url, body) => { calls.push(['POST', url, structuredClone(body)]); return {} },
    put: async (url, body) => { calls.push(['PUT', url, structuredClone(body)]); return [] },
  }
  const helper = { exports: {} }
  new Function('require', 'module', 'exports', fs.readFileSync(path.join(root, 'miniprogram/utils/recommendation.js'), 'utf8'))(() => api, helper, helper.exports)
  const deps = {
    '../../utils/api': api,
    '../../utils/recommendation': helper.exports,
    '../../utils/session': { requireLogin: () => true, hasSession: () => true, syncTabBar() {} },
    '../../utils/ui': { toast: text => toasts.push(text), haptic() {} },
    '../../utils/dish': { toCard: dish => ({ ...dish }), toCards: dishes => dishes, HOME_MOODS: [], HOME_MOOD_MAP: {} },
    '../../utils/format': { dateKey: () => '2026-10-25', greeting: () => '', addDays: date => date, WEEKDAYS: [] },
    '../../utils/media': { assetUrl: value => value, asArray: value => value || [] },
    '../../utils/theme': { page: value => { definition = value } },
  }
  vm.runInNewContext(fs.readFileSync(path.join(root, `miniprogram/pages/${name}/${name}.js`), 'utf8'), {
    require: name => deps[name], wx: { pageScrollTo() {} }, Date, Math, Set, clearTimeout, setTimeout,
  })
  const page = {
    data: structuredClone(definition.data),
    setData(patch, complete) { Object.assign(this.data, patch); complete?.() },
    selectComponent: () => null,
    createIntersectionObserver() {
      const observer = { relativeToViewport() { return this }, observe(selector, fire) { this.selector = selector; this.fire = fire }, disconnect() {} }
      observers.push(observer)
      return observer
    },
    _seen: [], _rejected: [], _recommendationId: '', _today: [],
  }
  for (const [key, value] of Object.entries(definition)) if (typeof value === 'function') page[key] = value.bind(page)
  return { page, api, calls, toasts, observers }
}

test('mini home reports only a visible card and skips never become dislikes', async () => {
  const { page, calls, observers } = mini('home')
  page.setRecommendations(result('first', [1, 2, 3]), 'lunch')
  assert.equal(calls.length, 0)
  observers[0].fire({ intersectionRatio: 0.25 })
  assert.equal(calls.length, 0)
  observers[0].fire({ intersectionRatio: 1 })
  await Promise.resolve()
  assert.equal(calls[0][1], '/recommendation-feedback')
  assert.equal(calls[0][2].event_type, 'impression')
  await page.changeRecommend()
  assert.equal(calls[1][2].event_type, 'skip')
  assert.equal(page.data.current.id, 2)
  assert.ok(calls.every(call => !['reject', 'dislike'].includes(call[2].event_type)))
})

test('mini home accumulates exclusions across batches and does not use catalog fallback on failure', async () => {
  const { page, api, calls } = mini('home')
  page.setRecommendations(result('first', [1, 2, 3]), 'dinner')
  let batch = 0
  api.post = async (url, body) => {
    calls.push(['POST', url, structuredClone(body)])
    if (url !== '/pick/smart') return {}
    batch++
    if (batch === 1) return result('second', [4, 5, 6])
    throw new Error('offline')
  }
  for (let i = 0; i < 6; i++) await page.changeRecommend()
  const requests = calls.filter(call => call[1] === '/pick/smart')
  assert.deepEqual(requests[0][2].exclude_dish_ids, [1, 2, 3])
  assert.deepEqual(requests[1][2].exclude_dish_ids, [1, 2, 3, 4, 5, 6])
  assert.equal(page.data.current.id, 6)
})

test('mini selected recommendation creates an independent plan and no duplicate accept event', async () => {
  const { page, calls } = mini('home')
  page.loadToday = async () => {}
  page.setRecommendations(result('first', [7]), 'lunch')
  await page.recordCurrent({ currentTarget: { dataset: { meal: 'lunch' } } })
  assert.equal(calls.length, 1)
  assert.equal(calls[0][1], '/meal-plans')
  assert.equal(calls[0][2].recommendation_id, 'first')
  assert.equal(calls[0][2].dish_id, 7)
})

test('mini recommendation officer distinguishes temporary refusal, dislike and undo', async () => {
  const { page, calls } = mini('assistant')
  page._recommendationId = 'advisor'
  page.setData({ items: [dish(1), dish(2)] })
  await page.reject({ currentTarget: { dataset: { id: 1 } } })
  await page.reject({ currentTarget: { dataset: { id: 2, kind: 'dislike' } } })
  await page.undoDislike()
  assert.deepEqual(calls.map(call => call[2].event_type), ['not_today', 'dislike', 'undo_dislike'])
  assert.equal(page.data.canUndoDislike, false)
})

test('mini tomorrow replaces plans per meal without deleting consumed records', async () => {
  const { page, calls } = mini('tomorrow')
  page._original = [{ id: 20, dish_id: 9, meal_type: 'lunch' }]
  page._plan = { lunch: [dish(1)], dinner: [dish(2)] }
  page.setData({ dateKey: '2026-10-26', hasSaved: true, hasChanges: true })
  page.render = () => {}
  await page.save()
  assert.deepEqual(calls.map(call => [call[0], call[1]]), [['PUT', '/meal-plans'], ['PUT', '/meal-plans']])
  assert.deepEqual(calls[0][2].dish_ids, [1])
  assert.deepEqual(calls[1][2].dish_ids, [2])
})

test('Web recommendation impressions wait for viewport visibility and deduplicate observations', async () => {
  const { JSDOM } = front('jsdom'), React = front('react'), ts = front('typescript')
  const dom = new JSDOM('<div id="root"></div>', { url: 'https://example.test' })
  const previous = { window: global.window, document: global.document, IntersectionObserver: global.IntersectionObserver, IS_REACT_ACT_ENVIRONMENT: global.IS_REACT_ACT_ENVIRONMENT }
  global.window = dom.window; global.document = dom.window.document; global.IS_REACT_ACT_ENVIRONMENT = true
  let observe
  global.IntersectionObserver = class { constructor(callback) { observe = callback } observe() {} disconnect() {} }
  const calls = []
  const stubs = { '@/api': { recommendationFeedbackApi: { send: async (...args) => calls.push(args) } } }
  const mod = { exports: {} }
  const code = ts.transpileModule(fs.readFileSync(path.join(root, 'frontend/src/lib/use-recommendation-impression.ts'), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  new Function('require', 'module', 'exports', code)(name => stubs[name] || front(name), mod, mod.exports)
  const { createRoot } = front('react-dom/client')
  function Card() { const ref = React.useRef(null); mod.exports.useRecommendationImpression(ref, 'visible-rec', 8); return React.createElement('div', { ref }, 'recipe') }
  const mounted = createRoot(document.querySelector('#root'))
  try {
    await React.act(async () => mounted.render(React.createElement(Card)))
    assert.equal(calls.length, 0)
    await React.act(async () => observe([{ isIntersecting: false }]))
    assert.equal(calls.length, 0)
    await React.act(async () => observe([{ isIntersecting: true }]))
    await React.act(async () => mounted.render(React.createElement(Card)))
    assert.deepEqual(calls, [['visible-rec', 8, 'impression']])
  } finally {
    await React.act(async () => mounted.unmount()); dom.window.close()
    for (const [key, value] of Object.entries(previous)) if (value === undefined) delete global[key]; else global[key] = value
  }
})

test('mini tomorrow preserves per-dish recommendation attribution through rendering and replacement', async () => {
  const { page, api, calls, observers } = mini('tomorrow')
  api.post = async (url, body) => {
    calls.push(['POST', url, structuredClone(body)])
    return { ...result('tomorrow-batch', [3, 4]), dishes: [dish(3), dish(4)] }
  }
  const picks = await page.request('lunch', 2, [])
  assert.equal(calls.length, 1)
  assert.equal(picks[0].recommendation_id, 'tomorrow-batch')
  page._original = []
  page._plan = { lunch: picks, dinner: [] }
  page.setData({ dateKey: '2026-10-26', targets: { lunch: 2, dinner: 0 } })
  page.render()
  assert.equal(calls.length, 1)
  assert.equal(observers.length, 2)
  observers[0].fire({ intersectionRatio: 1 })
  await Promise.resolve()
  assert.equal(calls[1][2].event_type, 'impression')
  await page.save()
  const saved = calls.find(call => call[0] === 'PUT' && call[2].meal_type === 'lunch')
  assert.deepEqual(saved[2].recommendation_ids, { 3: 'tomorrow-batch', 4: 'tomorrow-batch' })
})

test('mini recommendation officer starts a new session when meal or filters change', () => {
  const { page } = mini('assistant')
  page.setData({ meal: 'lunch' })
  assert.equal(page.buildRequest().session_id, undefined)
  page._recommendationId = 'lunch-snapshot'
  assert.equal(page.buildRequest().session_id, 'lunch-snapshot')
  page.setData({ meal: 'dinner' })
  assert.equal(page.buildRequest().session_id, undefined)
  page._recommendationId = 'dinner-snapshot'
  page.setData({ includeText: '鸡蛋' })
  assert.equal(page.buildRequest().session_id, undefined)
})
