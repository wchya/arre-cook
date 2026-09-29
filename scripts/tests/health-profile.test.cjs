const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')
const initial = { version: 1, active: true, goal: 'balanced', eating_pattern: 'mixed', allergies: ['花生'], dietary_exclusions: ['猪肉'], confirmed_at: '2026-09-29T00:00:00Z' }

function mini() {
 let definition, identity = 'alice'
 const calls = [], events = []
 const api = { token: () => identity, get: async () => structuredClone(initial), put: async (url, body) => { calls.push([url, body]); return { ...initial, ...body, version: body.version + 1 } }, delete: async url => { calls.push([url]); return { ...initial, version: 2, active: false, goal: '', eating_pattern: '', allergies: [], dietary_exclusions: [], confirmed_at: null } } }
 vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../../miniprogram/components/health-profile/index.js'), 'utf8'), { Component: d => { definition = d }, require: () => api })
 const component = { data: structuredClone(definition.data), setData(patch) { Object.assign(this.data, patch) }, triggerEvent(e) { events.push(e) } }
 for (const [key, value] of Object.entries(definition.methods)) component[key] = value.bind(component)
 definition.lifetimes.attached.call(component)
 return { component, api, calls, events, changeTokenOnly() { identity = "bob" }, switchUser() { identity = 'bob'; definition.pageLifetimes.show.call(component) }, detach() { definition.lifetimes.detached.call(component) } }
}

test('mini voluntary profile requires fresh consent and retains version and distinct exclusions', async () => {
 const { component: c, calls, events } = mini()
 await c.toggle()
 await c.saveProfile()
 assert.equal(calls.length, 0)
 c.onConsent({ detail: { value: ['confirmed'] } })
 c.onAllergies({ detail: { value: '虾、花生，牛奶' } })
 assert.equal(c.data.confirmed, false)
 c.onConsent({ detail: { value: ['confirmed'] } })
 await c.saveProfile()
 assert.equal(calls[0][1].version, 1)
 assert.equal(calls[0][1].confirmed, true)
 assert.deepEqual(Array.from(calls[0][1].allergies), ['虾', '花生', '牛奶'])
 assert.deepEqual(Array.from(calls[0][1].dietary_exclusions), ['猪肉'])
 assert.equal(c.data.confirmed, false)
 assert.deepEqual(events, ['changed'])
 const xml = fs.readFileSync(path.join(__dirname, '../../miniprogram/components/health-profile/index.wxml'), 'utf8')
 for (const match of xml.matchAll(/bind[a-z]+="([A-Za-z][A-Za-z0-9]+)"/g)) assert.equal(typeof c[match[1]], 'function', match[1])
})

test('mini profile conflict preserves edits and erasure is explicit and versioned', async () => {
 const { component: c, api, calls } = mini()
 await c.loadProfile()
 c.onAllergies({ detail: { value: '虾' } }); c.onConsent({ detail: { value: ['confirmed'] } })
 api.put = async () => { throw Object.assign(new Error('档案已更新'), { status: 409 }) }
 await c.saveProfile()
 assert.equal(c.data.allergies, '虾'); assert.equal(c.data.conflict, true); assert.equal(c.data.profile.version, 1)
 api.get = async () => ({ ...initial, version: 4 })
 await c.loadProfile()
 assert.equal(c.data.allergies, '花生'); assert.equal(c.data.confirmed, false)
 await c.clearProfile(); assert.equal(calls.length, 0)
 c.showErase(); await c.clearProfile()
 assert.equal(calls[0][0], '/health/profile?version=4')
 assert.equal(c.data.profile.active, false); assert.equal(c.data.allergies, '')
})

test('mini profile drops closed, detached and old-account responses', async () => {
 const { component: c, api, events, switchUser, detach } = mini()
 let finish
 api.get = () => new Promise(resolve => { finish = resolve })
 const loading = c.loadProfile(); c.toggle(); finish(initial); await loading
 assert.equal(c.data.profile, null)
 const second = c.loadProfile(); switchUser(); finish(initial); await second
 assert.equal(c.data.open, false); assert.equal(c.data.allergies, '')
 api.get = async () => initial; await c.loadProfile()
 c.onConsent({ detail: { value: ['confirmed'] } })
 api.put = () => new Promise(resolve => { finish = resolve })
 const saving = c.saveProfile(); detach(); finish({ ...initial, version: 2 }); await saving
 assert.equal(events.length, 0)
})

test('mini profile cannot submit the previous account draft before onShow fires', async () => {
 const { component: c, calls, changeTokenOnly } = mini()
 await c.loadProfile(); c.onConsent({ detail: { value: ['confirmed'] } })
 changeTokenOnly(); await c.saveProfile()
 assert.equal(calls.length, 0); assert.equal(c.data.profile, null); assert.equal(c.data.allergies, '')
})

test('Web profile confirms edits, handles conflicts and erasure, and discards late responses', async () => {
 const { createRequire } = require('node:module')
 const front = createRequire(path.resolve(__dirname, '../../frontend/package.json'))
 const { JSDOM } = front('jsdom')
 const dom = new JSDOM('<div id="root"></div>', { url: 'https://test.local' })
 const previous = {}
 for (const [key, value] of Object.entries({ window: dom.window, document: dom.window.document, IS_REACT_ACT_ENVIRONMENT: true })) { previous[key] = global[key]; global[key] = value }
 const React = front('react'), { createRoot } = front('react-dom/client'), ts = front('typescript')
 const calls = [], invalidated = []
 let epoch = 1, failure = false
 class ApiError extends Error { constructor(message) { super(message); this.status = 409 } }
 const healthApi = {
  profile: async () => structuredClone(initial),
  saveProfile: async body => { calls.push(body); if (failure) throw new ApiError('档案已更新'); return { ...initial, ...body, version: body.version + 1 } },
  clearProfile: async version => { calls.push({ clear: version }); return { ...initial, version: version + 1, active: false, allergies: [], dietary_exclusions: [], goal: '', eating_pattern: '', confirmed_at: null } },
 }
 const stubs = { '@tanstack/react-query': { useQueryClient: () => ({ invalidateQueries: async query => invalidated.push(query.queryKey[0]) }) }, '@/api': { healthApi, ApiError, errorMessage: e => e.message }, '@/api/client': { getSessionEpoch: () => epoch, isCurrentSession: e => e === epoch }, '@/components/RequestState': () => React.createElement('p', null, 'loading') }
 const mod = { exports: {} }
 const code = ts.transpileModule(fs.readFileSync(path.join(__dirname, '../../frontend/src/components/HealthProfilePanel.tsx'), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 } }).outputText
 new Function('require', 'module', 'exports', code)(name => stubs[name] || front(name), mod, mod.exports)
 const root = createRoot(dom.window.document.getElementById('root'))
 const button = text => Array.from(document.querySelectorAll('button')).find(el => el.textContent === text)
 const click = async el => { assert.ok(el); await React.act(async () => el.click()) }
 const checkbox = () => document.querySelector('input[type=checkbox]')
 try {
  await React.act(async () => root.render(React.createElement(mod.exports.default, { key: 1 })))
  await click(button('查看与填写'))
  assert.equal(button('确认保存档案').disabled, true)
  await click(checkbox())
  const select = document.querySelector('select')
  await React.act(async () => { select.value = 'regular_meals'; select.dispatchEvent(new dom.window.Event('change', { bubbles: true })) })
  assert.equal(checkbox().checked, false)
  await click(checkbox())
  failure = true; await click(button('确认保存档案'))
  assert.equal(calls[0].version, 1); assert.equal(calls[0].goal, 'regular_meals')
  assert.equal(document.querySelector('select').value, 'regular_meals')
  assert.ok(button('放弃本次编辑，载入最新档案'))
  healthApi.profile = async () => ({ ...initial, version: 3 })
  await click(button('放弃本次编辑，载入最新档案'))
  assert.equal(document.querySelector('select').value, 'balanced')
  assert.equal(checkbox().checked, false)
  failure = false; await click(checkbox()); await click(button('确认保存档案'))
  assert.equal(calls[1].version, 3); assert.ok(invalidated.includes('health-report')); assert.ok(invalidated.includes('pick'))
  await click(button('清除档案及历史')); await click(button('确认清除档案及历史'))
  assert.equal(calls[2].clear, 4)
  assert.equal(document.querySelector('textarea').value, '')
  assert.equal(button('清除档案及历史'), undefined)
  epoch++
  await click(checkbox()); await click(button('确认保存档案'))
  assert.equal(calls.length, 3, 'old draft must not be sent under a new session')
  assert.equal(document.querySelector('textarea'), null)
  let finish
  healthApi.profile = () => new Promise(resolve => { finish = resolve })
  await click(button('查看与填写')); await click(button('收起'))
  await React.act(async () => finish(initial))
  assert.equal(document.querySelector('textarea'), null)
  await click(button('查看与填写'))
  await React.act(async () => { epoch++; root.render(React.createElement(mod.exports.default, { key: 2 })); finish(initial) })
  assert.equal(document.querySelector('textarea'), null)
  assert.ok(button('查看与填写'))
 } finally {
  await React.act(async () => root.unmount()); dom.window.close()
  for (const key of Object.keys(previous)) { if (previous[key] === undefined) delete global[key]; else global[key] = previous[key] }
 }
})
