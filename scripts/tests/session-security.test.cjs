const fs = require('node:fs')
const path = require('node:path')
const assert = require('node:assert/strict')
const { createRequire } = require('node:module')
const { test } = require('node:test')
const root = path.resolve(__dirname, '../..')
const front = createRequire(path.join(root, 'frontend/package.json'))
const ts = front('typescript')
const { JSDOM } = front('jsdom')
function environment(storageDisabled = false) {
  const values = new Map()
  const check = () => { if (storageDisabled) throw new Error('storage disabled') }
  const shared = { getItem: k => { check(); return values.get(k) || null }, setItem: (k, v) => { check(); values.set(k, String(v)) }, removeItem: k => { check(); values.delete(k) } }
  function tab(url = 'https://cook.arrebyte.top/') {
    const win = new JSDOM('', { url }).window
    const sent = []
    function load(file, stubs = {}) {
      const module = { exports: {} }
      const code = ts.transpileModule(fs.readFileSync(path.join(root, file), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText
      new Function('require', 'module', 'exports', 'window', 'localStorage', 'sessionStorage', 'navigator', code)(n => n in stubs ? stubs[n] : front(n), module, module.exports, win, shared, win.sessionStorage, win.navigator)
      return module.exports
    }
    const client = load('frontend/src/api/client.ts')
    client.default.defaults.adapter = async config => {
      sent.push(config)
      return { data: { code: 0, data: { id: config.headers.Authorization === 'Bearer token-A' ? 1 : 2 } }, status: 200, statusText: 'OK', config, headers: {} }
    }
    const mini = load('frontend/src/lib/miniprogram.ts')
    const store = load('frontend/src/store/useAuthStore.ts', { '@/api': { meApi: { get: () => client.api('GET', '/me') } }, '@/api/client': client, '@/lib/miniprogram': mini }).useAuthStore
    return { win, client, mini, store, sent, load }
  }
  return { tab }
}
const flush = () => new Promise(resolve => setImmediate(resolve))
test('disabled browser storage preserves a tab-local login and logout', async () => {
  const a = environment(true).tab()
  a.store.getState().setSession('token-A', { id: 1 })
  assert.equal((await a.client.api('GET', '/me')).id, 1)
  a.win.dispatchEvent(new a.win.Event('focus'))
  assert.equal(a.client.getToken(), 'token-A')
  a.client.setToken(null)
  await a.client.api('GET', '/auth/options')
  assert.equal(a.sent.at(-1).headers.Authorization, undefined)
})
test('a delayed storage event cannot send an old tab mutation as the new account', async () => {
  const env = environment(), a = env.tab(), b = env.tab()
  a.store.getState().setSession('token-A', { id: 1 })
  b.store.getState().setSession('token-B', { id: 2 })
  assert.equal(a.store.getState().user.id, 1)
  await assert.rejects(a.client.api('POST', '/dishes', { name: 'private A draft' }), /账号状态已变化/)
  assert.equal(a.sent.filter(r => r.method === 'post').length, 0)
  await flush()
  assert.equal(a.store.getState().user.id, 2)
})
test('storage change clears the visible identity synchronously and rejects in-flight results', async () => {
  const env = environment(), a = env.tab(), b = env.tab()
  a.store.getState().setSession('token-A', { id: 1 })
  let complete
  const adapter = a.client.default.defaults.adapter
  a.client.default.defaults.adapter = config => config.url === '/dishes' ? new Promise(resolve => { complete = () => resolve({ data: { code: 0, data: ['A private data'] }, status: 200, config, headers: {} }) }) : adapter(config)
  const pending = a.client.api('GET', '/dishes')
  const rejected = assert.rejects(pending, /账号状态已变化/)
  b.store.getState().setSession('token-B', { id: 2 })
  a.win.dispatchEvent(new a.win.StorageEvent('storage', { key: 'ninimenu_token', oldValue: 'token-A', newValue: 'token-B' }))
  assert.equal(a.store.getState().user, null)
  assert.equal(a.store.getState().isLoggedIn, false)
  complete()
  await rejected
  await flush()
  assert.equal(a.store.getState().user.id, 2)
})
test('returning to the same token cannot validate a response from an older session epoch', async () => {
  const a = environment().tab()
  a.store.getState().setSession('token-A', { id: 1 })
  let complete
  a.client.default.defaults.adapter = config => new Promise(resolve => { complete = () => resolve({ data: { code: 0, data: 'old' }, status: 200, config, headers: {} }) })
  const pending = a.client.api('GET', '/dishes')
  const rejected = assert.rejects(pending, /账号状态已变化/)
  a.store.getState().setSession('token-B', { id: 2 })
  a.store.getState().setSession('token-A', { id: 1 })
  complete()
  await rejected
})
test('URL fragments cannot inject a session in a normal browser or a claimed mini-program link', async () => {
  for (const suffix of ['#token=token-B', '?from=mp#token=token-B']) {
    const a = environment().tab('https://cook.arrebyte.top/' + suffix)
    a.store.getState().setSession('token-A', { id: 1 })
    assert.equal(a.mini.isMiniProgram(), false)
    await a.store.getState().bootstrap()
    assert.equal(a.store.getState().user.id, 1)
    assert.equal(a.client.getToken(), 'token-A')
    assert.equal(a.win.location.hash, '')
    assert.equal(a.mini.isMiniProgram(), false)
  }
})
test('an unmounted account password mutation cannot restore its old account', () => {
  const a = environment().tab(), mutations = []
  a.store.getState().setSession('token-A', { id: 1 })
  const epoch = a.client.getSessionEpoch()
  const Component = a.load('frontend/src/pages/Account.tsx', {
    react: { ...front('react'), useState: initial => [initial, () => {}] },
    'react-router-dom': { useNavigate: () => () => {} },
    '@tanstack/react-query': { useMutation: options => { mutations.push(options); return {} } },
    '@/api': { meApi: {}, errorMessage: () => '' },
    '@/api/client': a.client,
    '@/components/PageHeader': { default: () => null },
    '@/store/useAuthStore': { useAuthStore: selector => selector(a.store.getState()) },
  }).default
  Component()
  a.store.getState().setSession('token-B', { id: 2 })
  mutations[0].onSuccess({ result: { token: 'replacement-A', user: { id: 1 } }, epoch })
  assert.equal(a.client.getToken(), 'token-B')
  assert.equal(a.store.getState().user.id, 2)
})
