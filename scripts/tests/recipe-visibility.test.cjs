const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('./miniprogram-vm.cjs')
const { test } = require('node:test')

const root = path.resolve(__dirname, '../..')
const settle = () => new Promise(resolve => setImmediate(resolve))

function catalogHarness() {
  const storage = new Map(), requests = [], navigations = []
  const timers = new Map()
  let nextTimer = 0
  const wx = {
    getStorageSync: key => storage.get(key),
    setStorageSync: (key, value) => storage.set(key, value),
    removeStorageSync: key => storage.delete(key),
    switchTab: value => navigations.push(value.url),
  }
  const api = { get: async (url, params = {}) => {
    requests.push({ url, params: { ...params } })
    if (url === '/dishes') return { items: params.scope === 'mine' ? [] : [{ id: 1, name: '公共菜谱', category: '川菜' }], total: params.scope === 'mine' ? 0 : 100 }
    if (url === '/dishes/category-counts') return { total: params.scope === 'mine' ? 0 : 100, categories: [] }
    return {}
  } }
  const deps = {
    api, session: { syncTabBar() {}, requireLogin: () => true }, ui: {},
    media: { asArray: value => Array.isArray(value) ? value : [] },
    dish: { toCard: value => value }, format: { dateKey: () => '2026-09-27' },
    theme: { bindRefresher: () => () => {} },
  }
  let definition
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/pages/dishes/dishes.js'), 'utf8'), {
    Page: value => { definition = value }, wx,
    require: name => deps[name.split('/').at(-1)],
    setTimeout: (callback, delay) => { timers.set(++nextTimer, { callback, delay }); return nextTimer },
    clearTimeout: id => timers.delete(id),
  })
  const page = { data: structuredClone(definition.data), setData(patch, callback) { Object.assign(this.data, patch); callback?.() } }
  for (const [name, fn] of Object.entries(definition)) if (typeof fn === 'function') page[name] = fn.bind(page)
  page.onLoad({})
  let tabDefinition
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/custom-tab-bar/index.js'), 'utf8'), {
    Component: value => { tabDefinition = value }, wx, getCurrentPages: () => [page],
  })
  const tab = { data: { selected: 1 } }
  tab.switchTab = tabDefinition.methods.switchTab.bind(tab)
  const runTimers = delay => {
    for (const [id, timer] of timers) if (timer.delay === delay) { timers.delete(id); timer.callback() }
  }
  return { page, tab, requests, storage, navigations, runTimers }
}

test('tapping the selected catalog tab leaves an empty private collection and restores public recipes', async () => {
  const h = catalogHarness()
  h.storage.set('ninimenu_dishes_scope', 'mine')
  h.page.onShow(); await settle()
  assert.equal(h.page.data.scope, 'mine')
  assert.equal(h.page.data.total, 0)
  h.tab.switchTab({ currentTarget: { dataset: { index: 1 } } }); await settle()
  assert.equal(h.page.data.scope, '')
  assert.equal(h.page.data.scopeLabel, '全部')
  assert.equal(h.page.data.total, 100)
  assert.equal(h.page.data.groups[0].items[0].name, '公共菜谱')
  assert.equal(h.requests.filter(r => r.url === '/dishes').at(-1).params.scope, undefined)
})

test('returning through the catalog tab clears private, category, taste and search filters before requesting', async () => {
  const h = catalogHarness()
  Object.assign(h.page.data, { scope: 'mine', category: '粤菜', taste: '甜', keyword: '不存在的菜' })
  h.tab.data.selected = 3
  h.tab.switchTab({ currentTarget: { dataset: { index: 1 } } })
  assert.equal(h.storage.get('ninimenu_dishes_scope'), 'all')
  h.page.onShow(); await settle()
  const params = h.requests.find(r => r.url === '/dishes').params
  for (const key of ['scope', 'category', 'taste', 'search']) assert.equal(params[key], undefined, key)
  assert.equal(h.page.data.total, 100)
  assert.equal(h.storage.has('ninimenu_dishes_scope'), false)
})

test('returning from a recipe detail preserves a deliberately selected scope', async () => {
  const h = catalogHarness()
  h.page.data.scope = 'family'
  h.page.data.scopeLabel = '家庭'
  h.page.onShow(); await settle()
  assert.equal(h.requests.find(r => r.url === '/dishes').params.scope, 'family')
  assert.equal(h.page.data.scope, 'family')
})

test('reopening the full catalog cancels a search still waiting for its debounce', async () => {
  const h = catalogHarness()
  h.page.onSearchInput({ detail: { value: '没有结果的关键词' } })
  h.storage.set('ninimenu_dishes_scope', 'all')
  h.page.onShow(); await settle()
  h.runTimers(320); await settle()
  assert.equal(h.page.data.keyword, '')
  assert.equal(h.requests.filter(r => r.url === '/dishes').at(-1).params.search, undefined)
  assert.equal(h.page.data.total, 100)
})
