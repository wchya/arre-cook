const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const { createRequire } = require('node:module')
const { test } = require('node:test')
const root = path.resolve(__dirname, '../..')
const front = createRequire(path.join(root, 'frontend/package.json'))
const ts = front('typescript')
const { JSDOM } = front('jsdom')
const key = 'ninimenu_appearance'

function setup({ stored = null, dark = false, disabled = false } = {}) {
  const win = new JSDOM('<meta name="theme-color" content="#fff">', { url: 'http://localhost/' }).window
  const listeners = new Set()
  const media = { matches: dark, addEventListener: (_, fn) => listeners.add(fn), removeEventListener: (_, fn) => listeners.delete(fn) }
  win.matchMedia = () => media
  if (stored !== null) win.localStorage.setItem(key, stored)
  if (disabled) Object.defineProperty(win, 'localStorage', { get() { throw new Error('Storage unavailable') } })
  function load(file, stubs = {}) {
    const module = { exports: {} }
    const source = ts.transpileModule(fs.readFileSync(path.join(root, file), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
    // Resolve localStorage lazily, preserving the browser's throwing getter behavior.
    const globals = new Proxy({}, { has: (_, name) => ['window', 'document', 'localStorage', 'getComputedStyle'].includes(name), get: (_, name) => name === 'window' ? win : name === 'document' ? win.document : name === 'localStorage' ? win.localStorage : win.getComputedStyle.bind(win) })
    new Function('scope', 'require', 'module', 'exports', `with(scope) { ${source} }`)(globals, name => stubs[name] || front(name), module, module.exports)
    return module.exports
  }
  const appearance = load('frontend/src/lib/appearance.ts')
  const state = load('frontend/src/store/useAppearanceStore.ts', { '@/lib/appearance': appearance })
  const cleanup = state.initializeAppearance()
  return { win, appearance, store: state.useAppearanceStore, cleanup, listeners, system(value) { media.matches = value; for (const fn of listeners) fn() } }
}

test('new users receive lime and follow the system without storing a forced mode', () => {
  const env = setup({ dark: true })
  assert.equal(env.win.document.documentElement.dataset.palette, 'lime')
  assert.equal(env.win.document.documentElement.dataset.mode, 'dark')
  assert.equal(env.store.getState().mode, 'system')
  assert.equal(env.win.localStorage.getItem(key), null)
  env.system(false)
  assert.equal(env.win.document.documentElement.dataset.mode, 'light')
  env.cleanup()
  assert.equal(env.listeners.size, 0)
})

test('palette and mode persist independently; explicit light overrides a dark system after reload', () => {
  const env = setup({ dark: true })
  env.store.getState().change({ palette: 'ocean' })
  env.store.getState().change({ mode: 'light' })
  const stored = env.win.localStorage.getItem(key)
  assert.deepEqual(JSON.parse(stored), { palette: 'ocean', mode: 'light' })
  const reload = setup({ dark: true, stored })
  assert.equal(reload.win.document.documentElement.dataset.mode, 'light')
  assert.equal(reload.win.document.documentElement.dataset.palette, 'ocean')
  reload.system(false)
  reload.system(true)
  assert.equal(reload.win.document.documentElement.dataset.mode, 'light')
  env.cleanup(); reload.cleanup()
})

test('corrupt or unknown preferences fall back without preventing startup', () => {
  for (const stored of ['invalid-json', 'null', '42', '{}', '{"palette":"constructor","mode":"invalid"}']) {
    const env = setup({ stored })
    assert.equal(env.store.getState().palette, 'lime')
    assert.equal(env.store.getState().mode, 'system')
    env.cleanup()
  }
})

test('storage unavailable still permits immediate theme changes and reports non-persistence', () => {
  const env = setup({ disabled: true })
  env.store.getState().change({ palette: 'berry', mode: 'dark' })
  assert.equal(env.win.document.documentElement.dataset.palette, 'berry')
  assert.equal(env.win.document.documentElement.dataset.mode, 'dark')
  assert.equal(env.store.getState().storageAvailable, false)
  env.cleanup()
})

test('other tabs update appearance, clearing storage resets it, and unrelated keys are ignored', () => {
  const env = setup()
  const send = (eventKey, newValue) => env.win.dispatchEvent(new env.win.StorageEvent('storage', { key: eventKey, newValue }))
  send(key, '{"palette":"peach","mode":"dark"}')
  assert.equal(env.store.getState().palette, 'peach')
  assert.equal(env.win.document.documentElement.dataset.mode, 'dark')
  send('ninimenu_token', null)
  assert.equal(env.store.getState().palette, 'peach')
  send(null, null)
  assert.equal(env.store.getState().palette, 'lime')
  assert.equal(env.store.getState().mode, 'system')
  env.cleanup()
  send(key, '{"palette":"ocean","mode":"dark"}')
  assert.equal(env.store.getState().palette, 'lime')
})

test('pre-paint bootstrap agrees with application parsing for every palette and mode', () => {
  const html = fs.readFileSync(path.join(root, 'frontend/index.html'), 'utf8')
  const bootstrap = html.match(/<script>([\s\S]*?)<\/script>/)[1]
  for (const palette of ['lime', 'peach', 'ocean', 'berry', 'toString']) {
    for (const mode of ['light', 'dark', 'system', 'invalid']) {
      for (const dark of [false, true]) {
        const env = setup({ stored: JSON.stringify({ palette, mode }), dark })
        const expected = { ...env.win.document.documentElement.dataset }
        new Function('document', 'localStorage', 'matchMedia', bootstrap)(env.win.document, env.win.localStorage, env.win.matchMedia)
        assert.deepEqual({ ...env.win.document.documentElement.dataset }, expected)
        env.cleanup()
      }
    }
  }
})
