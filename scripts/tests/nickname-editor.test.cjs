const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const root = path.resolve(__dirname, '../../miniprogram')

function load(file, globals) {
  vm.runInNewContext(fs.readFileSync(path.join(root, file), 'utf8'), globals, { filename: file })
}

function setup({ deferSheetUpdates = false } = {}) {
  let time = 0
  let timerId = 0
  const timers = new Map()
  const clock = {
    setTimeout(fn, delay) { timers.set(++timerId, { at: time + delay, fn }); return timerId },
    clearTimeout(id) { timers.delete(id) },
    tick(ms) {
      const end = time + ms
      for (;;) {
        const next = [...timers].filter(([, timer]) => timer.at <= end).sort((a, b) => a[1].at - b[1].at)[0]
        if (!next) break
        time = next[1].at
        timers.delete(next[0])
        next[1].fn()
      }
      time = end
    },
  }
  const app = { globalData: { isIOS: true, user: { nickname: '旧昵称' } } }
  const messages = []
  const api = { token: () => 'test', put: async (_, data) => ({ ...app.globalData.user, ...data }) }
  const wx = { hideKeyboard() {}, reLaunch() {} }
  let activePage
  const sheetTabsModule = { exports: {} }
  load('utils/sheet-tabs.js', { module: sheetTabsModule, getCurrentPages: () => activePage ? [activePage] : [] })
  const sheetTabs = sheetTabsModule.exports
  const sessionModule = { exports: {} }
  load('utils/session.js', { module: sessionModule, require: name => name === './sheet-tabs' ? sheetTabs : api, getApp: () => app, wx })
  const session = sessionModule.exports

  let tabDefinition
  load('custom-tab-bar/index.js', { Component: value => { tabDefinition = value } })
  const tab = { data: structuredClone(tabDefinition.data), setData(patch) { Object.assign(this.data, patch) } }

  let sheetDefinition
  load('components/sheet/index.js', { Component: value => { sheetDefinition = value }, require: () => sheetTabs, ...clock, wx, getApp: () => app })
  function createSheet() {
    const sheet = { data: structuredClone(sheetDefinition.data), events: [] }
    for (const [name, property] of Object.entries(sheetDefinition.properties)) sheet.data[name] = property.value
    for (const [name, fn] of Object.entries(sheetDefinition.methods)) sheet[name] = fn.bind(sheet)
    sheet.setData = (patch, callback) => { Object.assign(sheet.data, patch); if (callback) callback() }
    return sheet
  }
  const sheet = createSheet()
  const repeatSheet = createSheet()

  let pageDefinition
  const dependencies = {
    '../../utils/api': api,
    '../../utils/session': session,
    '../../utils/ui': { toast: message => messages.push(message), haptic() {} },
    '../../utils/media': { assetUrl: value => value || '' },
    '../../utils/theme': {},
  }
  load('pages/me/me.js', { Page: value => { pageDefinition = value }, require: name => dependencies[name], wx })
  const page = { data: structuredClone(pageDefinition.data), getTabBar: () => tab }
  activePage = page
  for (const [name, value] of Object.entries(pageDefinition)) if (typeof value === 'function') page[name] = value.bind(page)
  page.load = () => {}
  page.data.user = app.globalData.user
  page.setData = (patch) => {
    const previous = { ...page.data }
    Object.assign(page.data, patch)
    for (const [key, component] of [['nameOpen', sheet], ['repeatOpen', repeatSheet]]) {
      if (previous[key] === page.data[key]) continue
      const show = page.data[key]
      const update = () => {
        component.data.show = show
        sheetDefinition.observers.show.call(component, show)
      }
      if (deferSheetUpdates) clock.setTimeout(update, 0)
      else update()
    }
  }
  sheet.triggerEvent = (name) => {
    sheet.events.push(name)
    if (name === 'opened') page.onNameOpened()
    if (name === 'closed') page.onNameClosed()
    if (name === 'close') page.closeName()
  }
  repeatSheet.triggerEvent = (name) => {
    repeatSheet.events.push(name)
    if (name === 'closed') page.onRepeatClosed?.()
    if (name === 'close') page.closeRepeat()
  }
  page.onShow()
  return { page, sheet, repeatSheet, tab, api, app, messages, session, clock }
}

test('opening hides the actual tab component until the close animation completes', () => {
  const { page, tab, sheet, clock } = setup()
  page.openName()
  assert.equal(tab.data.hidden, true)
  assert.equal(sheet.data.visible, true)
  assert.equal(page.data.nameFocus, false)
  clock.tick(410)
  assert.equal(page.data.nameFocus, true)
  page.onNameKeyboardHeightChange({ detail: { height: 336 } })
  assert.equal(page.data.nameKeyboardHeight, 336)
  page.closeName()
  assert.equal(page.data.nameFocus, false)
  assert.equal(page.data.nameKeyboardHeight, 0)
  assert.equal(tab.data.hidden, true)
  clock.tick(379)
  assert.equal(tab.data.hidden, true)
  clock.tick(1)
  assert.equal(tab.data.hidden, false)
  assert.equal(sheet.data.visible, false)
})

test('a successful save updates the profile and restores the menu', async () => {
  const { page, app, tab, clock } = setup()
  page.openName()
  clock.tick(410)
  page.onNameInput({ detail: { value: ' 新昵称 ' } })
  await page.saveName()
  assert.equal(app.globalData.user.nickname, '新昵称')
  assert.equal(page.data.user.nickname, '新昵称')
  assert.equal(page.data.savingName, false)
  assert.equal(page.data.nameOpen, false)
  assert.equal(tab.data.hidden, true)
  clock.tick(380)
  assert.equal(tab.data.hidden, false)
})

test('a failed save keeps the editor open and allows retry without the menu overlapping', async () => {
  const { page, api, tab, messages } = setup()
  page.openName()
  api.put = async () => { throw new Error('网络暂时不可用') }
  await page.saveName()
  assert.equal(page.data.nameOpen, true)
  assert.equal(page.data.savingName, false)
  assert.equal(tab.data.hidden, true)
  assert.equal(messages.at(-1), '网络暂时不可用')
  api.put = async () => ({ nickname: '重试成功' })
  await page.saveName()
  assert.equal(page.data.user.nickname, '重试成功')
})

test('reopening during close cancels stale timers that could reveal the menu', () => {
  const { page, sheet, tab, clock } = setup()
  page.openName()
  clock.tick(410)
  page.closeName()
  clock.tick(100)
  page.openName()
  clock.tick(410)
  assert.equal(page.data.nameOpen, true)
  assert.equal(page.data.nameFocus, true)
  assert.equal(sheet.data.visible, true)
  assert.equal(tab.data.hidden, true)
})

test('mask and close button use the same dismissal path', () => {
  for (const method of ['onMask', 'onClose']) {
    const { page, sheet, tab, clock } = setup()
    page.openName()
    clock.tick(410)
    sheet[method]()
    clock.tick(380)
    assert.equal(page.data.nameOpen, false)
    assert.equal(tab.data.hidden, false)
  }
})

test('leaving the page restores the menu and late close events cannot switch the selected tab', () => {
  for (const method of ['onHide', 'onUnload']) {
    const { page, sheet, tab, session, clock } = setup()
    page.openName()
    clock.tick(410)
    page[method]()
    assert.equal(tab.data.hidden, false)
    assert.equal(page.data.nameOpen, false)
    session.syncTabBar(page, 0)
    clock.tick(380)
    assert.equal(tab.data.selected, 0)
    assert.equal(sheet.data.visible, false)
  }
})

test('repeat settings hide navigation before the sheet receives its open property', () => {
  const { page, repeatSheet, tab, clock } = setup({ deferSheetUpdates: true })
  page.openRepeat()
  assert.equal(tab.data.hidden, true)
  assert.equal(repeatSheet.data.visible, false)
  page.onShow()
  assert.equal(tab.data.hidden, true)
  clock.tick(410)
  assert.equal(repeatSheet.data.active, true)
  repeatSheet.onMask()
  clock.tick(379)
  assert.equal(tab.data.hidden, true)
  clock.tick(1)
  assert.equal(tab.data.hidden, false)
})

test('choosing repeat days saves the value and waits for dismissal before restoring navigation', async () => {
  const { page, repeatSheet, tab, api, clock } = setup()
  let saved
  api.put = async (url, data) => { saved = { url, data } }
  page.openRepeat()
  clock.tick(410)
  page.chooseRepeat({ currentTarget: { dataset: { value: '7' } } })
  await Promise.resolve()
  assert.equal(saved.url, '/settings')
  assert.equal(saved.data.settings.repeat_days, '7')
  assert.equal(page.data.repeatDays, '7')
  assert.equal(page.data.repeatOpen, false)
  assert.equal(repeatSheet.data.visible, true)
  assert.equal(tab.data.hidden, true)
  clock.tick(380)
  assert.equal(repeatSheet.data.visible, false)
  assert.equal(tab.data.hidden, false)
})

test('leaving repeat settings closes them so returning cannot retain a sheet over navigation', () => {
  for (const method of ['onHide', 'onUnload']) {
    const { page, repeatSheet, tab, session, clock } = setup()
    page.openRepeat()
    clock.tick(410)
    page[method]()
    assert.equal(page.data.repeatOpen, false)
    assert.equal(tab.data.hidden, false)
    session.syncTabBar(page, 0)
    clock.tick(380)
    assert.equal(tab.data.selected, 0)
    assert.equal(repeatSheet.data.visible, false)
    page.onShow()
    assert.equal(tab.data.hidden, false)
  }
})

test('reopening repeat settings or switching editors cannot reveal navigation mid-transition', () => {
  const { page, repeatSheet, tab, clock } = setup()
  page.openRepeat()
  clock.tick(410)
  page.closeRepeat()
  clock.tick(100)
  page.openRepeat()
  clock.tick(410)
  assert.equal(repeatSheet.data.active, true)
  assert.equal(tab.data.hidden, true)
  page.closeRepeat()
  page.openName()
  clock.tick(410)
  assert.equal(tab.data.hidden, true)
  page.closeName()
  clock.tick(380)
  assert.equal(tab.data.hidden, false)
})
