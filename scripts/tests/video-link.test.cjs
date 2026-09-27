const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { createRequire } = require('node:module')
const { test } = require('node:test')

const root = path.resolve(__dirname, '../..')
const requireFront = createRequire(path.join(root, 'frontend/package.json'))
const ts = requireFront('typescript')
const share = '【【饭店味！！黄焖辣子鸡保姆级教程】-哔哩哔哩】 https://b23.tv/lnFjJz8'
const canonical = 'https://www.bilibili.com/video/BV1Vo1zB2EZf/'
const web = { exports: {} }
const code = ts.transpileModule(fs.readFileSync(path.join(root, 'frontend/src/lib/video-link.ts'), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
vm.runInNewContext(code, { module: web, exports: web.exports, URL })

function miniVideo(wx = {}) {
  const module = { exports: {} }
  vm.runInNewContext(fs.readFileSync(path.join(root, 'miniprogram/utils/video.js'), 'utf8'), { module, wx })
  return module.exports
}

test('both clients open the normalized video URL from share messages and preserve the selected part', () => {
  for (const parse of [web.exports.videoLink, miniVideo().platform]) {
    for (const [raw, expected] of [
      [share, 'https://b23.tv/lnFjJz8'], [canonical, canonical],
      [canonical + '?p=2&share_source=copy', canonical + '?p=2'],
      ['抖音分享 https://v.douyin.com/abc123/。', 'https://v.douyin.com/abc123/'],
      ['https://www.douyin.com/?modal_id=123456', 'https://www.douyin.com/video/123456'],
    ]) assert.equal(parse(raw)?.url, expected, raw)
    for (const raw of [
      'javascript:alert(1)', 'https://b23.tv@evil.invalid/lnFjJz8', 'https://evil.b23.tv/lnFjJz8',
      'https://www.bilibili.com.evil.invalid/video/BV1Vo1zB2EZf/',
      canonical.replace('bilibili.com', 'bilibili.com:8080'), canonical + '?p=2&p=3',
      share + ' https://www.douyin.com/video/123',
    ]) assert.equal(parse(raw), null, raw)
  }
})

test('mini playback copies a clean usable link when direct opening is unavailable or rejected', () => {
  for (const direct of [false, true]) {
    const copied = [], opened = [], messages = []
    const wx = { setClipboardData: value => { copied.push(value.data); value.success() } }
    if (direct) wx.openUrl = value => { opened.push(value.url); value.fail() }
    const video = miniVideo(wx)
    if (!direct) assert.match(video.actionLabel(share), /复制链接到哔哩哔哩播放/)
    assert.equal(video.open(share, name => messages.push(name)), true)
    assert.deepEqual(copied, ['https://b23.tv/lnFjJz8'])
    assert.deepEqual(opened, direct ? ['https://b23.tv/lnFjJz8'] : [])
    assert.deepEqual(messages, ['哔哩哔哩'])
    assert.equal(video.open('javascript:alert(1)'), false)
    assert.equal(copied.length, 1)
  }
})

test('mini recipe pages keep a usable video action when metadata is unavailable', async () => {
  for (const name of ['dish', 'dish-edit']) {
    let definition
    const video = miniVideo()
    const api = { get: async () => { throw new Error('preview unavailable') } }
    vm.runInNewContext(fs.readFileSync(path.join(root, `miniprogram/pages/${name}/${name}.js`), 'utf8'), {
      Page: value => { definition = value },
      require: name => ({ video, api, media: { assetUrl: value => value || '' } })[name.split('/').at(-1)] || {},
    })
    const page = { data: { videoUrl: share }, setData(patch) { Object.assign(this.data, patch) } }
    for (const [key, value] of Object.entries(definition)) if (typeof value === 'function') page[key] = value.bind(page)
    const fallback = page.decorateMeta(null, share)
    assert.equal(fallback.url, 'https://b23.tv/lnFjJz8')
    assert.equal(fallback.supported, true)
    assert.equal(fallback.playable, false)
    assert.match(fallback.actionLabel, /复制链接到哔哩哔哩播放/)
    assert.equal(page.decorateMeta(null, 'javascript:alert(1)'), null)
    if (name === 'dish-edit') {
      await page.fetchVideoPreview()
      assert.equal(page.data.videoChecking, false)
      assert.equal(page.data.videoMeta.url, fallback.url)
    }
  }
})
