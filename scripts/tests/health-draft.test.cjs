const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')
const draft = { items: [{ dish_name: '鸡蛋', portion: '', evidence: '我吃了鸡蛋' }, { dish_name: '米饭', portion: '半碗', evidence: '半碗米饭' }] }
const status = { enabled: true, quota: { limit: 10, remaining: 10 } }
function mini() {
 let definition, identity = 'alice'
 const calls = [], events = []
 const api = { token: () => identity, get: async () => status, post: async (url, body) => { calls.push([url, body]); return structuredClone(draft) } }
 vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../../miniprogram/components/health-meal-draft/index.js'), 'utf8'), { Component: d => { definition = d }, require: () => api, Date, Math })
 const c = { data: structuredClone(definition.data), setData(patch) { Object.assign(this.data, patch) }, triggerEvent(e) { events.push(e) } }
 for (const [key, value] of Object.entries(definition.methods)) c[key] = value.bind(c)
 definition.lifetimes.attached.call(c)
 return { c, api, calls, events, changeToken: () => { identity = 'bob' }, show: () => definition.pageLifetimes.show.call(c), hide: () => definition.pageLifetimes.hide.call(c), detach: () => definition.lifetimes.detached.call(c) }
}
const input = value => ({ detail: { value } })
const itemInput = (index, field, value) => ({ currentTarget: { dataset: { index, field } }, detail: { value } })
async function prepare(c) { c.onText(input('我吃了鸡蛋和半碗米饭')); await c.parse(); c.onMeal(input(1)); c.onConsent(input(['confirmed'])) }

test('mini text draft remains editable and requires fresh consent before atomic confirmation', async () => {
 const { c, calls, events } = mini()
 await prepare(c)
 assert.equal(calls.length, 1); assert.match(calls[0][0], /parse$/); assert.equal(c.data.items[0].portion, '')
 c.onItem(itemInput(0, 'portion', '一个')); assert.equal(c.data.confirmed, false)
 await c.save(); assert.equal(calls.length, 1)
 c.onConsent(input(['confirmed'])); await c.save()
 assert.match(calls[1][0], /confirm$/); assert.equal(calls[1][1].meal_type, 'breakfast'); assert.equal(calls[1][1].confirmed, true)
 assert.equal(calls[1][1].items[0].portion, '一个'); assert.equal(calls[1][1].items[0].evidence, undefined)
 assert.equal(c.data.items.length, 0); assert.deepEqual(events, ['saved'])
 const xml = fs.readFileSync(path.join(__dirname, '../../miniprogram/components/health-meal-draft/index.wxml'), 'utf8')
 for (const match of xml.matchAll(/bind[a-z]+="([A-Za-z][A-Za-z0-9]+)"/g)) assert.equal(typeof c[match[1]], 'function', match[1])
})

test('mini ambiguous save failure freezes same submission and retry reuses exact request key', async () => {
 const { c, api } = mini(); await prepare(c)
 const bodies = []
 api.post = async (_, body) => { bodies.push(JSON.stringify(body)); if (bodies.length === 1) throw new Error('timeout'); return {} }
 await c.save(); assert.equal(c.data.submitted, true); assert.equal(c.data.saving, false)
 c.onText(input('别的食物')); c.onItem(itemInput(0, 'dish_name', '面包')); c.onMeal(input(3))
 assert.equal(c.data.items[0].dish_name, '鸡蛋'); assert.equal(c.data.mealIndex, 1)
 await c.save(); assert.equal(bodies[0], bodies[1]); assert.equal(c.data.submitted, false)
})

test('mini cancel, hide, detach and account switch suppress late parsing and abort native request', async () => {
 for (const action of ['cancel', 'hide', 'detach', 'switch']) {
  const { c, api, hide, detach, changeToken, show } = mini()
  let finish, aborted = 0
  api.post = async (_, __, options) => { options.onTask({ abort: () => aborted++ }); return new Promise(resolve => { finish = resolve }) }
  c.onText(input('我吃了鸡蛋')); const pending = c.parse()
  if (action === 'cancel') c.cancel()
  if (action === 'hide') hide()
  if (action === 'detach') detach()
  if (action === 'switch') { changeToken(); show() }
  finish(draft); await pending
  assert.equal(c.data.items.length, 0); assert.equal(aborted, 1)
 }
})

test('mini old draft cannot be confirmed under another token before page show', async () => {
 const { c, calls, changeToken } = mini(); await prepare(c); changeToken(); await c.save()
 assert.equal(calls.length, 1)
})

test('mini failure and empty response keep original text and do not emit saved', async () => {
 const { c, api, events } = mini(); c.onText(input('明天打算吃米饭'))
 api.post = async () => ({ items: [] }); await c.parse(); assert.match(c.data.message, /实际饮食/)
 api.post = async () => { throw new Error('超时') }; await c.parse()
 assert.equal(c.data.text, '明天打算吃米饭'); assert.match(c.data.message, /超时/); assert.equal(events.length, 0)
})

test('mini hiding during save preserves retry receipt and ignores late completion', async () => {
 const { c, api, hide, show, events } = mini(); await prepare(c)
 let finish
 api.post = async () => new Promise(resolve => { finish = resolve })
 const pending = c.save(); const key = c._submission.request_key
 hide(); show(); finish({}); await pending
 assert.equal(events.length, 0); assert.equal(c.data.submitted, true); assert.equal(c.data.saving, false)
 api.post = async (_, body) => { assert.equal(body.request_key, key); return {} }
 await c.save(); assert.deepEqual(events, ['saved'])
})

const { createRequire } = require('node:module')
const front = createRequire(path.resolve(__dirname, '../../frontend/package.json'))
test('Web draft edits reset consent and retry keeps exact body while account change blocks writes', async () => {
 const { JSDOM } = front('jsdom'), React = front('react'), ts = front('typescript')
 const dom = new JSDOM('<div id="root"></div>', { url: 'https://example.test' })
 const previous = { window: global.window, document: global.document, IS_REACT_ACT_ENVIRONMENT: global.IS_REACT_ACT_ENVIRONMENT }
 global.window = dom.window; global.document = dom.window.document; global.IS_REACT_ACT_ENVIRONMENT = true
 const { createRoot } = front('react-dom/client')
 let epoch = 1, fail = true
 let parseResponse = async () => structuredClone(draft)
 const submissions = [], invalidations = []
 const stubs = {
  '@/api': { ApiError: class ApiError extends Error {}, errorMessage: e => e.message, healthApi: { draftStatus: async () => status, parseDraft: (...args) => parseResponse(...args), confirmDraft: async body => { submissions.push(JSON.stringify(body)); if (fail) throw new Error('timeout'); return { entry_ids: [1, 2] } } } },
  '@/api/client': { getSessionEpoch: () => epoch, isCurrentSession: value => value === epoch },
  '@/lib/health-date': { healthDate: () => '2026-09-29' },
  '@tanstack/react-query': { useQueryClient: () => ({ invalidateQueries: async query => invalidations.push(query.queryKey[0]) }) },
 }
 const code = ts.transpileModule(fs.readFileSync(path.join(__dirname, '../../frontend/src/components/HealthMealDraftPanel.tsx'), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 } }).outputText
 const mod = { exports: {} }; new Function('require', 'module', 'exports', code)(name => stubs[name] || front(name), mod, mod.exports)
 const root = createRoot(document.querySelector('#root'))
 const button = text => [...document.querySelectorAll('button')].find(b => b.textContent === text)
 const click = text => React.act(async () => button(text).click())
 const change = (element, value) => React.act(async () => { const proto = element.tagName === 'TEXTAREA' ? dom.window.HTMLTextAreaElement.prototype : element.tagName === 'SELECT' ? dom.window.HTMLSelectElement.prototype : dom.window.HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto, 'value').set.call(element, value); element.dispatchEvent(new dom.window.Event(element.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true })) })
 try {
  await React.act(async () => root.render(React.createElement(mod.exports.default)))
  await click('打开文字记餐'); await change(document.querySelector('textarea'), '我吃了鸡蛋和半碗米饭'); await click('整理成草稿')
  assert.match(document.body.textContent, /尚未保存/); assert.equal(submissions.length, 0)
  await change(document.querySelector('select'), 'breakfast')
  await React.act(async () => document.querySelector('input[type=checkbox]').click())
  const name = document.querySelectorAll('input:not([type=date]):not([type=checkbox])')[0]
  await change(name, '煮鸡蛋'); assert.equal(document.querySelector('input[type=checkbox]').checked, false)
  await React.act(async () => document.querySelector('input[type=checkbox]').click())
  await click('确认保存这餐'); assert.equal(submissions.length, 1); assert.ok(button('重试确认保存')); assert.equal(name.disabled, true)
  fail = false; await click('重试确认保存'); assert.equal(submissions[0], submissions[1]); assert.deepEqual(invalidations, ['food-journal', 'health-report'])
  let finishParse, parseSignal
  parseResponse = async (_, signal) => { parseSignal = signal; return new Promise(resolve => { finishParse = resolve }) }
  await change(document.querySelector('textarea'), '我吃了鸡蛋'); await click('整理成草稿'); await click('停止整理')
  assert.equal(parseSignal.aborted, true)
  await React.act(async () => finishParse(draft)); assert.equal(document.querySelector('select'), null)
  parseResponse = async () => structuredClone(draft)
  await click('整理成草稿'); await change(document.querySelector('select'), 'dinner'); await React.act(async () => document.querySelector('input[type=checkbox]').click())
  epoch++; await click('确认保存这餐'); assert.equal(submissions.length, 2)
 } finally { await React.act(async () => root.unmount()); dom.window.close(); for (const key of Object.keys(previous)) { if (previous[key] === undefined) delete global[key]; else global[key] = previous[key] } }
})

test('mini definite validation rejection unlocks correction and requires new consent', async () => {
 const { c, api } = mini(); await prepare(c)
 api.post = async () => { throw Object.assign(new Error('日期不正确'), { status: 400 }) }
 await c.save(); assert.equal(c.data.submitted, false); assert.equal(c.data.confirmed, false); assert.equal(c._submission, null)
 c.onDate(input('2026-09-28')); assert.equal(c.data.date, '2026-09-28')
})
