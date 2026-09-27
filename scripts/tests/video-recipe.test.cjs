const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const { test } = require('node:test')

const root = path.resolve(__dirname, '../..')
const frontRequire = createRequire(path.join(root, 'frontend/package.json'))
const ts = frontRequire('typescript')
function loadTS(file, stubs = {}) {
  const full = path.resolve(root, file), module = { exports: {} }
  const code = ts.transpileModule(fs.readFileSync(full, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  new Function('require', 'module', 'exports', code)((name) => name in stubs ? stubs[name] : name.startsWith('.') ? loadTS(path.resolve(path.dirname(full), name + '.ts'), stubs) : frontRequire(name), module, module.exports)
  return module.exports
}
const blank = { name: '', ingredientsText: '', seasoningsText: '', stepsText: '', remark: '', cookTime: 0, importedRecipeJSON: '' }
const recipe = {
  name: '番茄炒蛋', cook_time: 0, remark: '',
  ingredients: [{ name: '番茄', amount: '两个', evidence: '番茄两个切块' }],
  seasonings: [{ name: '低钠 生抽', amount: '1 1/2 汤匙', evidence: '低钠 生抽 1 1/2 汤匙' }],
  steps: [{ text: '番茄切块\n鸡蛋打散，再炒熟。', time: 0, evidence: '番茄切块，鸡蛋打散' }],
}
const result = { recipe, source: { url: 'https://www.bilibili.com/video/BV1abCDefGh1/', method: 'subtitle', platform: 'bilibili', text: '番茄两个切块，鸡蛋打散，再炒熟。' } }

test('both clients fill only unchanged empty fields and preserve structured draft data', () => {
  for (const platform of ['web', 'mini']) {
    const importer = platform === 'web' ? loadTS('frontend/src/lib/recipe-import.ts') : require(path.join(root, 'miniprogram/utils/recipe-import.js'))
    const parser = platform === 'web' ? loadTS('frontend/src/lib/recipe-text.ts') : require(path.join(root, 'miniprogram/utils/recipe-text.js'))
    const patch = importer.recipeImportPatch(blank, blank, recipe)
    assert.equal(patch.name, recipe.name, platform)
    const originals = importer.readImportedRecipe(JSON.parse(JSON.stringify(patch)).importedRecipeJSON)
    assert.deepEqual(parser.parseIngredients(patch.seasoningsText, originals.seasonings), [{ name: '低钠 生抽', amount: '1 1/2 汤匙' }], platform)
    assert.deepEqual(parser.parseSteps(patch.stepsText, originals.steps), [{ text: recipe.steps[0].text, time: 0 }], platform)
    const edited = { ...blank, name: '用户自己的菜名', stepsText: '用户写的步骤', cookTime: 25 }
    const preserved = importer.recipeImportPatch(blank, edited, recipe)
    assert.equal(preserved.name, undefined); assert.equal(preserved.stepsText, undefined); assert.equal(preserved.cookTime, undefined)
    const clearedDuringRequest = importer.recipeImportPatch(blank, blank, recipe, ['ingredientsText'])
    assert.equal(clearedDuringRequest.ingredientsText, undefined, 'typed then cleared must stay empty')
    const clearedExisting = importer.recipeImportPatch({ ...blank, name: '原菜名' }, blank, recipe)
    assert.equal(clearedExisting.name, undefined, 'an explicitly cleared existing field must stay empty')
  }
})

test('draft restoration keeps imported amounts and step structure without exposing another account', () => {
  const store = new Map(), module = { exports: {} }
  const wx = { getStorageSync: key => store.get(key), setStorageSync: (key, value) => store.set(key, structuredClone(value)), removeStorageSync: key => store.delete(key) }
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/utils/recipe-draft.js'), 'utf8'), { module, wx })
  const fields = { ...blank, category: '川菜', mealType: 'all', difficulty: 'easy', tasteList: [], imageUrl: '', images: [], videoUrl: result.source.url, tags: [], sortOrder: 0 }
  const importer = require(path.join(root, 'miniprogram/utils/recipe-import.js'))
  const value = { ...fields, ...importer.recipeImportPatch(fields, fields, recipe) }
  assert.equal(module.exports.write(3, 0, value), true)
  assert.equal(module.exports.read(4, 0), null)
  assert.equal(module.exports.read(3, 0).value.importedRecipeJSON, value.importedRecipeJSON)
  global.localStorage = { getItem: key => JSON.stringify(store.get(key)) }
  const drafts = loadTS('frontend/src/lib/recipe-draft.ts')
  assert.equal(drafts.readRecipeDraft(drafts.recipeDraftKey(3, 0, 'user')).value.importedRecipeJSON, value.importedRecipeJSON)
})

function miniStream() {
  let request, chunk, token = 'first', aborts = 0, logout = 0
  const api = { token: () => token }, module = { exports: {} }
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/utils/video-recipe.js'), 'utf8'), {
    module, require: name => name === './api' ? api : { logout: () => logout++ },
    getApp: () => ({ globalData: { apiBase: 'https://cook.example/api' } }),
    wx: { request: value => { request = value; return { abort: () => aborts++, onChunkReceived: fn => { chunk = fn } } } },
    Uint8Array, ArrayBuffer,
  })
  return { ...module.exports, get request() { return request }, get aborts() { return aborts }, get logout() { return logout }, rotate: () => { token = 'second' }, push(bytes) { chunk({ data: Uint8Array.from(bytes).buffer }) } }
}
const frame = (event, data) => `event: ${event}\r\ndata: ${JSON.stringify(data)}\r\n\r\n`

test('mini stream preserves split Chinese characters, waits for done and never submits a duplicate request', async () => {
  const stream = miniStream(), progress = []
  const request = stream.extract({ url: result.source.url }, event => progress.push(event))
  const bytes = Buffer.from(frame('status', { message: '正在提炼🍳' }) + frame('recipe', result) + frame('done', {}))
  for (let i = 0; i < bytes.length; i++) stream.push(bytes.subarray(i, i + 1))
  let resolved = false; request.promise.then(() => { resolved = true })
  await Promise.resolve(); assert.equal(resolved, false, 'must await HTTP completion')
  stream.request.success({ statusCode: 200, data: new ArrayBuffer(0) })
  assert.equal((await request.promise).recipe.name, recipe.name)
  assert.equal(progress[0].data.message, '正在提炼🍳')
  assert.equal(stream.aborts, 0)
})

test('mini stream supports non-chunked libraries and rejects truncation, errors, cancellation and stale tokens', async () => {
  let stream = miniStream(), request = stream.extract({}, () => {})
  const text = frame('recipe', result) + frame('done', {})
  stream.request.success({ statusCode: 200, data: Uint8Array.from(Buffer.from(text)).buffer })
  assert.equal((await request.promise).recipe.name, recipe.name)
  for (const mode of ['truncated', 'error', 'cancel', 'token']) {
    stream = miniStream(); request = stream.extract({}, () => {})
    const rejected = assert.rejects(request.promise)
    if (mode === 'cancel') request.abort()
    else if (mode === 'token') { stream.rotate(); stream.push(Buffer.from(frame('recipe', result))) }
    else {
      stream.push(Buffer.from(frame('recipe', result) + (mode === 'error' ? frame('error', { message: '审核未通过' }) : '')))
      stream.request.success({ statusCode: 200, data: new ArrayBuffer(0) })
    }
    await rejected
    assert.equal(stream.logout, 0, 'stale request must not log out the new account')
  }
})

test('Web stream requires complete approved result and handles split UTF-8, errors and aborts', async () => {
  const oldWindow = global.window, oldFetch = global.fetch
  global.window = { setTimeout, clearTimeout, dispatchEvent() {} }
  class ApiError extends Error { constructor(message, code, status, data) { super(message); Object.assign(this, { code, status, data }) } }
  const stream = loadTS('frontend/src/api/video-recipe.ts', { './client': { getToken: () => 'fixture', ApiError } })
  try {
    for (const mode of ['ok', 'truncated', 'error', 'cancel']) {
      const text = frame('status', { message: '提炼🍳' }) + frame('recipe', result) + (mode === 'truncated' ? '' : mode === 'error' ? frame('error', { message: '审核未通过' }) : frame('done', {}))
      global.fetch = async () => new Response(new ReadableStream({ start(controller) { for (const byte of Buffer.from(text)) controller.enqueue(Uint8Array.of(byte)); controller.close() } }), { headers: { 'content-type': 'text/event-stream' } })
      const controller = new AbortController()
      if (mode === 'cancel') controller.abort()
      const promise = stream.extractVideoRecipe({ url: result.source.url }, () => {}, controller.signal)
      if (mode === 'ok') assert.equal((await promise).recipe.name, recipe.name)
      else await assert.rejects(promise)
    }
  } finally { global.window = oldWindow; global.fetch = oldFetch }
})

function miniEditor() {
  let definition, resolve, reject, saved, aborted = 0
  const store = new Map()
  const wx = { getStorageSync: key => store.get(key), setStorageSync: (key, value) => store.set(key, value), removeStorageSync: key => store.delete(key) }
  const draftModule = { exports: {} }
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/utils/recipe-draft.js'), 'utf8'), { module: draftModule, wx })
  const api = { token: () => 'fixture', get: async () => ({ enabled: true, quota: { remaining: 19 } }), post: async (_path, data) => { saved = data; return { id: 1 } } }
  const deps = {
    api, ui: { toast() {}, haptic() {} }, video: require(path.join(root, 'miniprogram/utils/video.js')), session: {}, dish: {},
    media: { asArray: value => Array.isArray(value) ? value : [], assetUrl: value => value },
    'recipe-text': require(path.join(root, 'miniprogram/utils/recipe-text.js')),
    'recipe-import': require(path.join(root, 'miniprogram/utils/recipe-import.js')),
    'recipe-draft': draftModule.exports,
    'video-recipe': { extract() { return { promise: new Promise((yes, no) => { resolve = yes; reject = no }), abort() { aborted++; const error = Error('aborted'); error.name = 'AbortError'; reject(error) } } } },
  }
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/pages/dish-edit/dish-edit.js'), 'utf8'), {
    Page: value => { definition = value }, require: name => deps[name.split('/').at(-1)], wx,
    setTimeout: () => 0, clearTimeout() {}, getCurrentPages: () => [],
  })
  const page = { data: structuredClone(definition.data), setData(patch, callback) { Object.assign(this.data, patch); callback?.() } }
  for (const [key, value] of Object.entries(definition)) if (typeof value === 'function') page[key] = value.bind(page)
  Object.assign(page.data, { loading: false, videoImportAvailable: true, videoUrl: result.source.url })
  page._draftUserId = 7; page.fetchVideoPreview = () => {}
  return { page, api, finish: () => resolve(structuredClone(result)), get saved() { return saved }, get aborted() { return aborted } }
}

test('mini editor protects edits made during extraction and saves imported structure only after Save', async () => {
  const harness = miniEditor(), { page } = harness
  const pending = page.extractVideo()
  page.onName({ detail: { value: '我的名字' } })
  page.onIngredients({ detail: { value: '刚才输入' } })
  page.onIngredients({ detail: { value: '' } })
  harness.finish(); await pending
  assert.equal(page.data.name, '我的名字')
  assert.equal(page.data.ingredientsText, '')
  assert.match(page.data.stepsText, /鸡蛋打散/)
  assert.equal(harness.saved, undefined)
  await page.save()
  assert.deepEqual(JSON.parse(harness.saved.seasonings), [{ name: '低钠 生抽', amount: '1 1/2 汤匙' }])
  assert.equal(JSON.parse(harness.saved.steps)[0].text, recipe.steps[0].text)
})

test('mini editor cancels and ignores old results when the video URL changes or the page is hidden', async () => {
  for (const mode of ['url', 'hide']) {
    const harness = miniEditor(), pending = harness.page.extractVideo()
    if (mode === 'url') harness.page.onVideo({ detail: { value: 'https://v.douyin.com/changed/' } })
    else harness.page.onHide()
    await pending
    assert.equal(harness.aborted, 1)
    assert.equal(harness.page.data.name, '')
    assert.equal(harness.page.data.stepsText, '')
    assert.equal(harness.page.data.videoImportBusy, false)
  }
})

test('mini extraction quota is not overwritten by an older status response', async () => {
  const harness = miniEditor()
  let deliver
  harness.api.get = () => new Promise(resolve => { deliver = resolve })
  const olderStatus = harness.page.loadVideoImportStatus()
  const pending = harness.page.extractVideo()
  harness.page.applyVideoQuota({ remaining: 4 })
  deliver({ enabled: true, quota: { remaining: 20 } })
  await olderStatus
  assert.match(harness.page.data.videoQuotaText, /剩余 4 次/)
  harness.api.get = async () => ({ enabled: true, quota: { remaining: 4 } })
  harness.page.cancelVideoImport()
  await pending
})
