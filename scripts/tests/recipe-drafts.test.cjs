const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('./miniprogram-vm.cjs')
const { createRequire } = require('node:module')
const { test } = require('node:test')

const root = path.resolve(__dirname, '../..')
const frontRequire = createRequire(path.join(root, 'frontend/package.json'))
const ts = frontRequire('typescript')
const { JSDOM } = frontRequire('jsdom')
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost', pretendToBeVisual: true })
global.window = dom.window
global.document = dom.window.document
Object.defineProperty(global, 'navigator', { value: dom.window.navigator, configurable: true })
global.IS_REACT_ACT_ENVIRONMENT = true
const React = frontRequire('react')
const { createRoot } = frontRequire('react-dom/client')
const { act } = React

function loadTS(file, stubs = {}, cache = new Map()) {
  const full = path.resolve(root, file)
  if (cache.has(full)) return cache.get(full).exports
  const module = { exports: {} }; cache.set(full, module)
  const code = ts.transpileModule(fs.readFileSync(full, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const requireModule = name => {
    if (name in stubs) return stubs[name]
    if (name.startsWith('.')) return loadTS(path.resolve(path.dirname(full), name + '.ts'), stubs, cache)
    return frontRequire(name)
  }
  new Function('require', 'module', 'exports', 'clearTimeout', code)(requireModule, module, module.exports, window.clearTimeout.bind(window))
  return module.exports
}

function storage() {
  const values = new Map()
  return { values, fail: false, getItem: key => values.get(key) || null, setItem(key, value) { if (this.fail) throw Error('quota'); values.set(key, value) }, removeItem: key => values.delete(key) }
}
const initial = { name: '', category: '川菜', mealType: 'all', difficulty: 'easy', tasteList: [], cookTime: 15, ingredientsText: '', seasoningsText: '', stepsText: '', remark: '', imageUrl: '', images: [], videoUrl: '', tags: [], sortOrder: 0 }
const { useRecipeDraft } = loadTS('frontend/src/lib/use-recipe-draft.ts')
const { recipeDraftKey, readRecipeDraft } = loadTS('frontend/src/lib/recipe-draft.ts')

async function mount(userId = 7, recipeId = 0) {
  const host = document.createElement('div'); document.body.appendChild(host)
  const renderer = createRoot(host)
  let current
  function Form() {
    const [value, setValue] = React.useState(initial)
    const draft = useRecipeDraft(userId, recipeId, 'user', value, setValue)
    current = { value, setValue, draft }
    return React.createElement('div', null, draft.status)
  }
  await act(async () => renderer.render(React.createElement(Form)))
  return { get current() { return current }, async edit(patch) { await act(async () => current.setValue(value => ({ ...value, ...patch }))) }, async unmount() { await act(async () => renderer.unmount()); host.remove() } }
}

test('Web draft flushes full text when leaving before the debounce fires', async () => {
  global.localStorage = storage()
  const form = await mount()
  const steps = '小火炖煮，注意搅拌。'.repeat(80)
  await form.edit({ name: '番茄牛腩', stepsText: steps })
  await form.unmount()
  const saved = readRecipeDraft(recipeDraftKey(7, 0, 'user'))
  assert.equal(saved.value.stepsText, steps)
  assert.equal(saved.value.name, '番茄牛腩')
})

test('Web offers recovery without overwriting it, and isolates users and recipe IDs', async () => {
  global.localStorage = storage()
  const key = recipeDraftKey(7, 0, 'user')
  localStorage.setItem(key, JSON.stringify({ version: 1, savedAt: Date.now(), value: { ...initial, name: '待恢复' } }))
  let form = await mount(8)
  assert.equal(form.current.draft.recovery, null)
  await form.unmount()
  form = await mount(7, 99)
  assert.equal(form.current.draft.recovery, null)
  await form.unmount()
  form = await mount()
  assert.equal(form.current.draft.recovery.value.name, '待恢复')
  await form.unmount()
  assert.equal(readRecipeDraft(key).value.name, '待恢复')
  form = await mount()
  await act(async () => form.current.draft.resume())
  assert.equal(form.current.value.name, '待恢复')
  await form.unmount()
  assert.equal(readRecipeDraft(key).value.name, '待恢复')
})

test('Web clears drafts on success and never recreates them during unmount', async () => {
  global.localStorage = storage()
  const form = await mount()
  await form.edit({ name: '已保存' })
  await act(async () => window.dispatchEvent(new window.Event('pagehide')))
  const key = recipeDraftKey(7, 0, 'user')
  assert.ok(readRecipeDraft(key))
  await act(async () => form.current.draft.clear())
  await form.unmount()
  assert.equal(readRecipeDraft(key), null)
})

test('Web removes a reverted draft and reports storage failures', async () => {
  global.localStorage = storage()
  const form = await mount()
  await form.edit({ name: '暂存' })
  await act(async () => window.dispatchEvent(new window.Event('pagehide')))
  await form.edit({ name: '' })
  await act(async () => window.dispatchEvent(new window.Event('pagehide')))
  assert.equal(readRecipeDraft(recipeDraftKey(7, 0, 'user')), null)
  localStorage.fail = true
  await form.edit({ name: '不能静默丢失' })
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 500)) })
  assert.match(form.current.draft.status, /未能保存/)
  await form.unmount()
})

function miniHarness(store = new Map(), userId = 7) {
  const messages = [], timers = new Map(); let timerId = 0
  const wx = { getStorageSync: key => store.get(key), setStorageSync: (key, value) => store.set(key, structuredClone(value)), removeStorageSync: key => store.delete(key), navigateBack() {}, switchTab() {} }
  const draftModule = { exports: {} }
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/utils/recipe-draft.js'), 'utf8'), { module: draftModule, wx })
  const drafts = draftModule.exports
  const api = { get: async () => ({}), post: async () => ({}), put: async () => ({}) }
  const deps = { api, ui: { toast: msg => messages.push(msg), haptic() {} }, media: { assetUrl: value => value, asArray: value => Array.isArray(value) ? value : [] }, video: {}, session: { requireLogin: () => true, currentUser: async () => ({ id: userId }) }, dish: { tasteTags: value => (value || '').split(',').filter(Boolean) }, 'recipe-draft': drafts, 'recipe-text': require(path.join(root, 'miniprogram/utils/recipe-text.js')), 'recipe-import': require(path.join(root, 'miniprogram/utils/recipe-import.js')), 'video-recipe': {} }
  let definition
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/pages/dish-edit/dish-edit.js'), 'utf8'), {
    Page: value => { definition = value }, require: name => deps[name.split('/').at(-1)], wx,
    getApp: () => ({ navMetrics: () => ({ navHeight: 88 }) }), getCurrentPages: () => [],
    setTimeout: fn => { timers.set(++timerId, fn); return timerId }, clearTimeout: id => timers.delete(id),
  })
  const page = { data: structuredClone(definition.data), setData(patch, callback) { Object.assign(this.data, patch); callback?.() } }
  for (const [key, value] of Object.entries(definition)) if (typeof value === 'function') page[key] = value.bind(page)
  page.fetchVideoPreview = () => {}
  return { page, drafts, api, store, messages }
}

test('mini program persists on hide/unload, recovers text and clears only after successful save', async () => {
  const state = miniHarness(); await state.page.onLoad({})
  state.page.onName({ detail: { value: '私房炖菜' } })
  state.page.onSteps({ detail: { value: '这是一段很长的步骤。'.repeat(80) } })
  state.page.onHide()
  assert.ok(state.drafts.read(7, 0).value.stepsText.length > 500)
  state.page.onUnload()
  const other = miniHarness(state.store, 8); await other.page.onLoad({})
  assert.equal(other.page.data.recovery, null)
  const restored = miniHarness(state.store); await restored.page.onLoad({})
  assert.equal(restored.page.data.recovery.value.name, '私房炖菜')
  restored.page.resumeDraft()
  restored.api.post = async () => { throw Error('网络不可用') }
  await restored.page.save()
  assert.equal(restored.page.data.saving, false)
  assert.ok(restored.drafts.read(7, 0))
  restored.api.post = async () => ({})
  await restored.page.save()
  restored.page.onUnload()
  assert.equal(restored.drafts.read(7, 0), null)
  other.page.onUnload()
})

test('mini program rejects malformed recipe links and can re-save a reverted draft', async () => {
  const invalid = miniHarness(); await invalid.page.onLoad({ id: 'not-an-id' })
  assert.match(invalid.page.data.error, /链接无效/)
  assert.equal(invalid.page._draftReady, undefined)
  const { page, drafts } = miniHarness(); await page.onLoad({})
  for (const name of ['临时菜', '', '临时菜']) { page.onName({ detail: { value: name } }); page.flushDraft() }
  assert.equal(drafts.read(7, 0).value.name, '临时菜')
  page.onUnload()
})

test('both clients display the same legal content', () => {
  let definition
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/pages/legal/legal.js'), 'utf8'), { Page: value => { definition = value } })
  const { legalContent } = loadTS('frontend/src/lib/legal-content.ts')
  for (const type of ['privacy', 'terms']) {
    let data; definition.onLoad.call({ setData: value => { data = value } }, { type })
    assert.equal(JSON.stringify(data), JSON.stringify(legalContent[type]))
  }
})

test('both editors preserve step photos, multiline instructions and ingredient amounts', () => {
  const clients = [loadTS('frontend/src/lib/recipe-text.ts'), require(path.join(root, 'miniprogram/utils/recipe-text.js'))]
  const steps = [{ text: '先焯水\n再洗净', time: 0.5, image: '/uploads/step-1.webp' }, { text: '放入锅中慢炖', time: 40, image: '/uploads/step-2.webp' }]
  const ingredients = [{ name: '低钠 生抽', amount: '1 1/2 汤匙' }, '少量温水']
  for (const parser of clients) {
    assert.deepEqual(parser.parseIngredients(parser.formatIngredients(ingredients), ingredients), ingredients)
    assert.deepEqual(parser.parseSteps(parser.formatSteps(steps), steps), steps)
    const simple = [{ text: '焯水', image: '/uploads/step-1.webp' }, { text: '慢炖', time: 40, image: '/uploads/step-2.webp' }]
    const moved = parser.parseSteps('1. 慢炖 (30分钟)\n2. 焯水 (0.5分钟)', simple)
    assert.equal(moved[0].image, '/uploads/step-2.webp')
    assert.equal(moved[0].time, 30)
    assert.equal(moved[1].image, '/uploads/step-1.webp')
    assert.equal(moved[1].time, 0.5)
  }
})

test('both calendars retrieve every page within the selected date range', async () => {
  for (const platform of ['web', 'mini']) {
    const requests = []
    async function read(params) {
      requests.push(params)
      const page = Number(params.page || 1)
      return { items: Array.from({ length: page === 1 ? 100 : 35 }, (_, index) => ({ id: (page - 1) * 100 + index + 1 })), total: 135, page, page_size: 100 }
    }
    let records
    if (platform === 'web') {
      records = loadTS('frontend/src/api/index.ts', { './client': { api: (_method, _path, params) => read(params), default: {} } }).recordsApi
    } else {
      const module = { exports: {} }
      vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/utils/records.js'), 'utf8'), { module, require: () => ({ get: (_path, params) => read(params) }) })
      records = module.exports
    }
    const result = await records.forDates('2026-08-01', '2026-08-31')
    assert.equal(result.items.length, 135, platform)
    assert.equal(requests.length, 2)
    requests.forEach(params => { assert.equal(params.date_from, '2026-08-01'); assert.equal(params.date_to, '2026-08-31') })
  }
})

test('an unavailable network preserves the session and stale requests cannot replace a newer account', async () => {
  let token = 'old'; let reject, resolve
  class ApiError extends Error { constructor(status) { super('request failed'); this.status = status } }
  const meApi = { get: () => new Promise((yes, no) => { resolve = yes; reject = no }) }
  const { useAuthStore } = loadTS('frontend/src/store/useAuthStore.ts', {
    '@/api': { meApi }, '@/api/client': { ApiError, getToken: () => token, setToken: value => { token = value } },
    '@/lib/miniprogram': { consumeTokenFromURL: () => null, isMiniProgram: () => false, backToMiniProgramLogin: async () => {} },
  })
  const first = useAuthStore.getState().bootstrap(); reject(Error('network')); await first
  assert.equal(token, 'old'); assert.equal(useAuthStore.getState().status, 'unavailable')
  const second = useAuthStore.getState().bootstrap()
  useAuthStore.getState().setSession('new', { id: 8, nickname: '新账号' })
  resolve({ id: 7, nickname: '旧账号' }); await second
  assert.equal(useAuthStore.getState().user.id, 8)
  const third = useAuthStore.getState().refresh()
  await useAuthStore.getState().logout()
  resolve({ id: 8 }); await third
  assert.equal(useAuthStore.getState().user, null)
})
