const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

function setup() {
 let definition, identity = 'alice'
 const calls = [], messages = []
 const api = { token: () => identity, get: async () => [], post: async (...args) => calls.push(['post', ...args]), put: async (...args) => calls.push(['put', ...args]), delete: async (...args) => calls.push(['delete', ...args]) }
 const deps = {
  '../../utils/api': api,
  '../../utils/session': { requireLogin: () => true },
  '../../utils/ui': { haptic() {}, toast: text => messages.push(text), confirm: async () => true },
  '../../utils/format': { dateKey: () => '2026-09-28', addDays: d => d, relativeDate: d => d, weekday: () => '周一' },
  '../../utils/media': { dishCover: () => '' }, '../../utils/dish': { dishEmoji: () => '' },
 }
 vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../../miniprogram/pages/diary/diary.js'), 'utf8'), { Page: d => { definition = d }, require: name => deps[name], wx: { stopPullDownRefresh() {} }, Date, Math })
 const page = { data: structuredClone(definition.data), setData(patch, done) { Object.assign(this.data, patch); done?.() } }
 for (const [key, value] of Object.entries(definition)) if (typeof value === 'function') page[key] = value.bind(page)
 return { page, api, calls, messages, switchUser: () => { identity = 'bob' } }
}

const report = { from: '2026-09-22', to: '2026-09-28', period_days: 7, logged_days: 1, complete_days: 0, meal_event_count: 1, item_count: 1, days: [{ date: '2026-09-28', status: 'partial', fingerprint: 'abc', item_count: 1, meal_count: 2, meal_event_count: 1, evidence: [{ source: 'journal', id: 7, meal_type: 'lunch', dish_name: '鸡蛋', possible_duplicate_ids: [3] }] }], plans: [], recommendations: [] }

test('mini report preserves evidence and confirms exact fingerprint', async () => {
 const { page, calls } = setup()
 page.applyReport(report)
 assert.equal(page.data.selectedDay.evidence[0].duplicateId, 3)
 page.loadReport = async () => {}
 await page.confirmDay()
 await new Promise(resolve => setImmediate(resolve))
 assert.equal(calls[0][1], '/health/days/2026-09-28/status')
 assert.equal(calls[0][2].fingerprint, 'abc')
 assert.equal(calls[0][2].complete, true)
})

test('mini editing and copying preserve portions but copying clears source link', async () => {
 const { page, calls } = setup()
 page.applyJournal([{ id: 4, meal_date: '2026-09-28', meal_type: 'lunch', dish_name: '鸡蛋', cuisine: '', notes: '', portion: '2 个', linked_record_id: 9, food_groups: ['protein'] }])
 page.editEntry({ currentTarget: { dataset: { id: 4 } } })
 assert.equal(page.data.editId, 4)
 assert.equal(page.data.formPortion, '2 个')
 page.load = async () => {}
 await page.saveEntry()
 assert.equal(calls[0][0], 'put')
 assert.equal(calls[0][1], '/food-journal/4')
 page.editEntry({ currentTarget: { dataset: { id: 4, copy: true } } })
 assert.equal(page.data.editId, null)
 assert.equal(page.data.formLinked, null)
 assert.equal(page.data.formPortion, '2 个')
})

test('mini retries reuse request key and preserve failed form', async () => {
 const { page, api } = setup()
 const keys = []
 api.post = async (_, data) => { keys.push(data.request_key); throw new Error('offline') }
 page.openForm(); page.setData({ formDish: '米饭' })
 await page.saveEntry(); await page.saveEntry()
 assert.equal(page.data.showForm, true)
 assert.equal(page.data.formDish, '米饭')
 assert.equal(keys.length, 2)
 assert.equal(keys[0], keys[1])
})

test('mini ignores report actions finishing after account switch', async () => {
 const { page, messages, switchUser } = setup()
 let finish
 const pending = new Promise(resolve => { finish = resolve })
 const result = page.reportAction(() => pending, 'old account result')
 switchUser(); finish(); await result
 assert.equal(messages.length, 0)
})

test('mini menu uses plan endpoint, not consumed records', async () => {
 const { page, calls } = setup()
 page.loadReport = async () => {}
 page.onPlanMeal({ detail: { value: 0 } })
 page.acceptPlan({ currentTarget: { dataset: { id: 8 } } })
 await new Promise(resolve => setImmediate(resolve))
 assert.equal(calls[0][1], '/health/plans')
 assert.equal(calls[0][2].meal_type, 'lunch')
})

test('all diary WXML event handlers exist', () => {
 const { page } = setup()
 const xml = fs.readFileSync(path.join(__dirname, '../../miniprogram/pages/diary/diary.wxml'), 'utf8')
 for (const match of xml.matchAll(/(?:bind|catch)(?::?[a-z]+)="([A-Za-z][A-Za-z0-9]+)"/g)) assert.equal(typeof page[match[1]], 'function', match[1])
})

test('mini nutrition converts kJ, preserves missing versus zero and submits actual volume', async () => {
 const { page, api, calls } = setup()
 let food
 api.post = async (url, body) => { calls.push(['post', url, body]); if (url === '/health/foods') return food = { ...body, id: 12, version: 1 } }
 api.get = async () => food ? [food] : []
 page.openForm(); page.setData({ formDish: '牛奶' })
 page.openLabel({ currentTarget: { dataset: {} } })
 page.onLabelName({ detail: { value: '包装牛奶' } })
 page.onLabelSource({ detail: { value: '包装标签' } })
 page.onLabelUnit({ detail: { value: 1 } })
 page.onLabelEnergyUnit({ detail: { value: 1 } })
 for (const [key, value] of [['energy_kcal', '418.4'], ['fat_g', '0']]) page.onLabelNutrient({ currentTarget: { dataset: { key } }, detail: { value } })
 await page.saveEntry()
 assert.equal(calls.length, 0, 'label draft blocks journal submit')
 await page.saveLabel()
 assert.ok(Math.abs(calls[0][2].nutrients.energy_kcal - 100) < 1e-9)
 assert.equal(calls[0][2].nutrients.fat_g, 0)
 assert.equal(calls[0][2].nutrients.fiber_g, null)
 page.onNutritionAmount({ detail: { value: '250' } })
 page.onPortionSource({ detail: { value: 1 } })
 page.load = async () => {}
 await page.saveEntry()
 assert.equal(calls[1][2].nutrition_food_id, 12)
 assert.equal(calls[1][2].nutrition_amount, 250)
 assert.equal(calls[1][2].nutrition_unit, 'ml')
 assert.equal(calls[1][2].portion_source, 'estimated')
})

test('mini label edit sends version and pending save cannot switch journal drafts', async () => {
 const { page, api, calls } = setup()
 const food = { id: 12, name: '牛奶', version: 3, basis_unit: 'ml', food_state: 'as_sold', source_reference: '标签', nutrients: { energy_kcal: 100, fat_g: 0 } }
 page.setData({ showForm: true, formDish: '原记录', formNutritionFood: food })
 page.openLabel({ currentTarget: { dataset: { edit: true } } })
 let finish
 api.put = async (url, body) => { calls.push(['put', url, body]); return new Promise(resolve => { finish = resolve }) }
 api.get = async () => [food]
 const saving = page.saveLabel()
 page.closeForm(); page.openForm()
 assert.equal(page.data.showForm, true)
 assert.equal(page.data.formDish, '原记录')
 assert.equal(calls[0][1], '/health/foods/12')
 assert.equal(calls[0][2].version, 3)
 finish({ ...food, version: 4 }); await saving
 assert.equal(page.data.formNutritionFood.version, 4)
 assert.equal(page.data.labelSaving, false)
})

test('mini comparison preserves zero and null, exposes paired dates and resets on period change', () => {
 const { page } = setup()
 page.applyReport({ ...report, comparison: { from: '2026-09-15', to: '2026-09-21', nutrients: [
  { code: 'fat_g', status: 'available', delta: 0, previous_average: 0, current_average: 0, current_dates: ['2026-09-28'], previous_dates: ['2026-09-21'] },
  { code: 'fiber_g', status: 'insufficient_data', delta: null, current_dates: [], previous_dates: [] },
 ] } })
 assert.equal(page.data.comparisonMetrics[0].deltaText, '0')
 assert.equal(page.data.comparisonMetrics[1].deltaText, '')
 assert.equal(page.data.comparisonMetrics[0].datePairs[0].previous, '2026-09-21')
 page.toggleComparison({ currentTarget: { dataset: { code: 'fat_g' } } })
 assert.equal(page.data.comparisonExpanded, 'fat_g')
 page.applyReport(report)
 assert.equal(page.data.comparisonMetrics.length, 0)
 assert.equal(page.data.comparisonExpanded, '')
})

test('Web comparison renders zero deltas and suppresses insufficient sample results', () => {
 const { createRequire } = require('node:module')
 const front = createRequire(path.resolve(__dirname, '../../frontend/package.json'))
 const ts = front('typescript'), React = front('react'), { renderToStaticMarkup } = front('react-dom/server')
 const mod = { exports: {} }
 const stubs = { 'react-router-dom': { useNavigate: () => () => {} }, '@tanstack/react-query': { useQueryClient: () => ({}) }, 'react-hot-toast': {}, '@/api': {}, '@/api/client': {}, '@/lib/health-date': { healthDate: () => '2026-09-28' } }
 const code = ts.transpileModule(fs.readFileSync(path.resolve(__dirname, '../../frontend/src/components/HealthReportPanel.tsx'), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 } }).outputText
 new Function('require', 'module', 'exports', code)(name => stubs[name] || front(name), mod, mod.exports)
 const data = { ...report, days: [], food_group_days: {}, insights: [], plan_actions: [], method_notes: [], nutrients: [], comparison: { from: '2026-09-15', to: '2026-09-21', nutrients: [{ code: 'fat_g', label: '脂肪', unit: 'g', status: 'available', delta: 0, current_average: 0, previous_average: 0, matched_days: 4, required_days: 4, current_dates: ['2026-09-28'], previous_dates: ['2026-09-21'], reason: '配对日期说明' }] } }
 const render = () => renderToStaticMarkup(React.createElement(mod.exports.default, { report: data, onRecord() {} })).replace(/<[^>]*>/g, '')
 assert.match(render(), /差值 0 g\/天/)
 assert.match(render(), /2026-09-21 → 2026-09-28/)
 Object.assign(data.comparison.nutrients[0], { status: 'insufficient_data', delta: null, current_average: null, previous_average: null, reason: '样本不足' })
 assert.doesNotMatch(render(), /差值/)
 assert.match(render(), /样本不足/)
 const catalog = { dataset: '历史目录', version: 'v1', record_id: '旧编号', license: '原许可', reviewed_by: '核验甲', reviewed_at: '2026-01-01', url: 'https://example.org/original' }
 data.days = [{ ...report.days[0], evidence: [{ ...report.days[0].evidence[0], possible_duplicate_ids: [], nutrition: { amount: 50, unit: 'g', food_version: 1, recipe: { yield_g: 200, ingredients: [{ food_id: 20, food_name: '原料', amount: 100, unit: 'g', food_version: 1, source_reference: catalog.url, catalog }] } } }] }]
 assert.match(render(), /历史目录 · v1 · 旧编号/)
 assert.match(render(), /核验甲（2026-01-01）/)
 assert.match(render(), /许可：原许可/)
 assert.match(render(), /查看原始来源/)
 data.portion_coverage = { measured_items: 1, standard_items: 2, estimated_items: 3, text_only_items: 4, unknown_items: 5 }
 assert.match(render(), /标准份量估算 2 项/)
 assert.match(render(), /仅文字份量 4 项/)
 assert.match(render(), /份量未知 5 项/)
})

test('mini mixed recipe requires confirmation, snapshots label versions and uses estimated portions', async () => {
 const { page, api, calls } = setup()
 const labels = [1,2].map(id => ({ id, source: 'package_label', name: '原料'+id, version: id, basis_unit: 'g', food_state: 'ready_to_eat' }))
 let recipe
 api.get = async () => [...labels, ...(recipe ? [recipe] : [])]
 api.post = async (url, body) => { calls.push(['post',url,body]); if (url === '/health/recipes') return recipe = { id: 3, name: body.name, source: 'retained_mixture', basis_unit:'g', food_state:'ready_to_eat', recipe: { yield_g:200, ingredients:[] }, version:1 } }
 await page.loadNutritionFoods()
 page.openRecipe({ currentTarget: { dataset: {} } })
 page.setData({ recipeName:'混合早餐',recipeSource:'测试说明',recipeYield:'200',formDish:'混合早餐' })
 for (let row=0;row<2;row++) {
  page.onRecipeFood({currentTarget:{dataset:{row}},detail:{value:row+1}})
  page.onRecipeAmount({currentTarget:{dataset:{row}},detail:{value:'100'}})
 }
 await page.saveRecipe(); await page.saveEntry()
 assert.equal(calls.length,0)
 page.onRecipeConfirm({detail:{value:['confirmed']}})
 await page.saveRecipe()
 assert.equal(calls[0][1],'/health/recipes')
 assert.equal(calls[0][2].ingredients[1].food_version,2)
 assert.equal(calls[0][2].method,'unheated_all_retained')
 assert.equal(page.data.formPortionSource,'estimated')
 page.onPortionSource({detail:{value:0}})
 assert.equal(page.data.formPortionSource,'estimated')
 assert.equal(page.data.recipeFoods.length,2,'recipes cannot be selected as ingredients')
 page.onNutritionAmount({detail:{value:'50'}})
 page.load=async()=>{}
 await page.saveEntry()
 assert.equal(calls[1][2].nutrition_food_id,3)
 assert.equal(calls[1][2].nutrition_amount,50)
})

test('mini recipe failure preserves draft and stale account response cannot replace new form', async () => {
 const { page, api, switchUser }=setup()
 page.setData({recipeFoods:[{id:1,version:1,basis_unit:'g',food_state:'as_sold'}],recipeName:'草稿',recipeYield:'200',recipeConfirmed:true,recipeOpen:true,recipeRows:[{index:1,amount:'100'},{index:1,amount:'100'}]})
 api.post=async()=>{throw Error('offline')}
 await page.saveRecipe()
 assert.equal(page.data.recipeName,'草稿')
 assert.equal(page.data.recipeOpen,true)
 assert.equal(page.data.labelSaving,false)
 let finish
 api.post=async()=>new Promise(resolve=>{finish=resolve})
 const pending=page.saveRecipe()
 switchUser();page.setData({formDish:'新账号',formNutritionFood:null})
 finish({id:99});await pending
 assert.equal(page.data.formNutritionFood,null)
 assert.equal(page.data.formDish,'新账号')
})

test('mini catalog ignores out-of-order and closed search responses', async () => {
 const { page, api } = setup()
 const pending = []
 api.get = url => new Promise(resolve => pending.push({ url, resolve }))
 page.setData({ catalogOpen: true, catalogQuery: '旧名称' })
 const old = page.searchCatalog()
 page.onCatalogQuery({ detail: { value: '新名称' } })
 const fresh = page.searchCatalog()
 pending[1].resolve([{ id: 2, name: '新名称', food_state: 'raw' }]); await fresh
 pending[0].resolve([{ id: 1, name: '旧名称', food_state: 'raw' }]); await old
 assert.equal(page.data.catalogResults[0].id, 2)
 const closed = page.searchCatalog()
 page.closeCatalog()
 pending[2].resolve([{ id: 3 }]); await closed
 assert.equal(page.data.catalogResults[0].id, 2)
})

test('mini catalog adoption uses a personal copy and blocks premature journal submission', async () => {
 const { page, api, calls } = setup()
 const food = { id: 20, name: '标准食物', source: 'standard_food', basis_unit: 'g', food_state: 'raw', version: 1, catalog: { dataset: 'test', version: 'v1' } }
 api.post = async (url, body) => { calls.push(['post', url, body]); return food }
 api.get = async () => [food]
 page.openForm(); page.setData({ formDish: food.name, catalogOpen: true })
 await page.saveEntry()
 assert.equal(calls.length, 0)
 await page.adoptCatalog({ currentTarget: { dataset: { id: 7 } } })
 assert.equal(calls[0][1], '/health/catalog/7/adopt')
 assert.equal(page.data.formNutritionFood.id, 20)
 assert.equal(page.data.recipeFoods[0].id, 20, 'standard food can be used as a mixture ingredient')
 page.onNutritionAmount({ detail: { value: '50' } })
 page.load = async () => {}
 await page.saveEntry()
 assert.equal(calls[1][2].nutrition_food_id, 20)
 assert.equal(calls[1][2].nutrition_amount, 50)
 assert.equal(calls[1][2].food_state, 'raw')
})

test('mini catalog adoption cannot populate another account', async () => {
 const { page, api, switchUser } = setup()
 let resolve
 api.post = () => new Promise(done => { resolve = done })
 const pending = page.adoptCatalog({ currentTarget: { dataset: { id: 7 } } })
 switchUser()
 page.setData({ formNutritionFood: null, formDish: '新账号' })
 resolve({ id: 20 }); await pending
 assert.equal(page.data.formNutritionFood, null)
 assert.equal(page.data.formDish, '新账号')
})

test('mini standard serving submits quantity only and cannot claim measured weight', async () => {
 const { page, api, calls } = setup()
 const food = { id: 20, name: '标准食物', source: 'standard_food', basis_unit: 'g', food_state: 'raw', version: 1, portions: [{ key: 'bowl', label: '一平碗', amount: 150, reference: '核验称量' }] }
 api.get = async () => [food]
 page.openForm(); page.setData({ formDish: food.name })
 await page.loadNutritionFoods()
 page.selectNutritionFood({ detail: { value: 1 } })
 page.chooseStandardPortion({ currentTarget: { dataset: { key: 'bowl' } } })
 page.onStandardPortionCount({ detail: { value: '0.5' } })
 page.onPortionSource({ detail: { value: 0 } })
 assert.equal(page.data.formPortionSource, 'estimated')
 page.load = async () => {}
 await page.saveEntry()
 assert.equal(calls[0][2].nutrition_portion_key, 'bowl')
 assert.equal(calls[0][2].nutrition_portion_count, 0.5)
 assert.equal(calls[0][2].nutrition_amount, null)
 page.chooseStandardPortion({ currentTarget: { dataset: {} } })
 assert.equal(page.data.formPortionKey, '')
 assert.equal(page.data.formStandardPortion, null)
 assert.equal(page.data.formPortionCount, '')
 page.chooseStandardPortion({ currentTarget: { dataset: { key: 'bowl' } } })
 page.selectNutritionFood({ detail: { value: 0 } })
 assert.equal(page.data.formStandardPortion, null)
})

test('Web standard serving switches inputs, marks estimate and clears stale quantity on food change', async () => {
 const { createRequire } = require('node:module')
 const front = createRequire(path.resolve(__dirname, '../../frontend/package.json'))
 const { JSDOM } = front('jsdom')
 const dom = new JSDOM('<div id="root"></div>', { url: 'https://test.local' })
 const previous = {}
 for (const [key, value] of Object.entries({ window: dom.window, document: dom.window.document, IS_REACT_ACT_ENVIRONMENT: true })) { previous[key] = global[key]; global[key] = value }
 const React = front('react'), { createRoot } = front('react-dom/client'), ts = front('typescript')
 const foods = [{ id: 20, name: '样本', source: 'standard_food', basis_unit: 'g', food_state: 'raw', catalog: {}, portions: [{ key: 'bowl', label: '一平碗', amount: 150, reference: '已核验份量来源' }] }, { id: 21, name: '包装', basis_unit: 'ml', food_state: 'as_sold' }]
 const stubs = { './NutritionCatalogSearch': () => null, './NutritionRecipeEditor': () => null, '@tanstack/react-query': { useQuery: () => ({ data: foods }), useQueryClient: () => ({}) }, 'react-hot-toast': {}, '@/api': { healthApi: {} }, '@/api/client': {} }
 const mod = { exports: {} }
 const code = ts.transpileModule(fs.readFileSync(path.resolve(__dirname, '../../frontend/src/components/NutritionEntryFields.tsx'), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 } }).outputText
 new Function('require', 'module', 'exports', code)(name => stubs[name] || front(name), mod, mod.exports)
 let current
 const noBusy = () => {}
 function Harness() {
  const [value, setValue] = React.useState({ nutrition_mode: 'replace', nutrition_food_id: 20, nutrition_amount: 90, portion_source: 'measured' })
  current = value
  return React.createElement(mod.exports.default, { value, onChange: setValue, onBusyChange: noBusy })
 }
 const root = createRoot(dom.window.document.getElementById('root'))
 try {
  await React.act(async () => root.render(React.createElement(Harness)))
  const selectFor = text => Array.from(dom.window.document.querySelectorAll('label')).find(label => label.textContent.startsWith(text)).querySelector('select')
  const serving = selectFor('食用量填写方式')
  await React.act(async () => { serving.value = 'bowl'; serving.dispatchEvent(new dom.window.Event('change', { bubbles: true })) })
  assert.equal(current.nutrition_amount, undefined)
  assert.equal(current.nutrition_portion_key, 'bowl')
  assert.equal(current.portion_source, 'estimated')
  assert.equal(selectFor('份量依据').disabled, true)
  assert.match(dom.window.document.body.textContent, /已核验份量来源/)
  assert.match(dom.window.document.body.textContent, /吃了几份“ 一平碗 ”|吃了几份“一平碗”/)
  const food = selectFor('选择我的标签或混合配方')
  await React.act(async () => { food.value = '21'; food.dispatchEvent(new dom.window.Event('change', { bubbles: true })) })
  assert.equal(current.nutrition_portion_key, undefined)
  assert.equal(current.nutrition_portion_count, undefined)
  assert.equal(current.nutrition_amount, undefined)
  assert.equal(current.nutrition_unit, 'ml')
 } finally {
  await React.act(async () => root.unmount())
  dom.window.close()
  for (const key of Object.keys(previous)) { if (previous[key] === undefined) delete global[key]; else global[key] = previous[key] }
 }
})
