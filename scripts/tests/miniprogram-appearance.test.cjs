const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const root = path.resolve(__dirname, '../../miniprogram')
const palettes = require(path.join(root, 'utils/theme-palettes'))
function harness({ raw, dark = false, failStorage = false } = {}) {
  const storage = new Map(raw === undefined ? [] : [['ninimenu_appearance', raw]])
  const native = [], pages = []
  let listener, definition
  const wx = {
    getStorageSync: k => { if (failStorage) throw Error('unavailable'); return storage.get(k) },
    setStorageSync: (k,v) => { if (failStorage) throw Error('full'); storage.set(k,v) },
    getAppBaseInfo: () => ({ theme: dark ? 'dark' : 'light' }),
    onThemeChange: fn => { assert.equal(listener, undefined); listener = fn },
    setBackgroundColor: p => native.push(p.backgroundColor), setNavigationBarColor() {}, setBackgroundTextStyle() {},
  }
  const module = { exports: {} }
  vm.runInNewContext(fs.readFileSync(path.join(root, 'utils/theme.js'), 'utf8'), {
    module, require: () => palettes, wx, Page: d => { definition = d }, getCurrentPages: () => pages,
  })
  const theme = module.exports
  function component(file, data = {}) {
    let d
    vm.runInNewContext(fs.readFileSync(path.join(root, file), 'utf8'), {
      Component: value => { d = value }, wx, getApp: () => ({ globalData: {} }),
      require: name => name.endsWith('/theme-palettes') ? palettes : name.endsWith('/theme') ? theme : name.endsWith('/icons') ? require(path.join(root, 'utils/icons')) : { lock() {}, unlock() {} },
      setTimeout, clearTimeout,
    })
    const instance = { data: { ...structuredClone(d.data), ...Object.fromEntries(Object.entries(d.properties || {}).map(([k,v]) => [k,v.value])), ...data }, updates:0,
      setData(p, cb) { Object.assign(this.data,p); this.updates++; cb?.() }, triggerEvent() {},
    }
    for (const [key, fn] of Object.entries(d.methods || {})) instance[key] = fn.bind(instance)
    d.lifetimes.attached.call(instance)
    instance.detach = () => d.lifetimes.detached.call(instance)
    return instance
  }
  return { theme, storage, native, pages, component,
    system(value) { dark = value === 'dark'; listener({ theme:value }) },
    register(options) { theme.page(options); return definition },
  }
}
test('all four palettes match Web source and text/action pairs remain readable in both modes', () => {
  const h = harness(), css = fs.readFileSync(path.resolve(root, '../frontend/src/themes.css'), 'utf8')
  const luminance = hex => hex.slice(1).match(/../g).map(v=>parseInt(v,16)/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((a,v,i)=>a+v*[.2126,.7152,.0722][i],0)
  const contrast = (a,b) => { a=luminance(a); b=luminance(b); return (Math.max(a,b)+.05)/(Math.min(a,b)+.05) }
  for (const [name, values] of Object.entries(palettes)) {
    const block = css.match(new RegExp('\\[data-palette="'+name+'"\\] \\{([\\s\\S]*?)\\n\\}'))[1]
    for (const [key,value] of Object.entries(values)) assert.ok(block.includes(`--palette-${key}: ${value};`),name+key)
    for (const mode of ['light','dark']) {
      const p = h.theme.set({palette:name,mode}), t=p.tokens
      assert.ok(contrast(t.text,t.card)>=4.5, name+mode+' body')
      assert.ok(contrast(t['on-primary'],t['primary-action'])>=4.5,name+mode+' button')
      assert.ok(!p.style.includes('undefined'))
    }
  }
})
test('manual mode ignores OS changes, system mode follows them and restores persisted selection', () => {
  const h = harness({ raw:'{"palette":"ocean","mode":"dark"}' })
  assert.equal(h.theme.name(),'dark')
  h.system('dark'); h.system('light'); assert.equal(h.theme.name(),'dark')
  h.theme.set({mode:'system'}); assert.equal(h.theme.name(),'light')
  h.system('dark'); assert.equal(h.theme.name(),'dark')
  assert.deepEqual(JSON.parse(h.storage.get('ninimenu_appearance')),{palette:'ocean',mode:'system'})
  assert.equal(harness({raw:h.storage.get('ninimenu_appearance'),dark:true}).theme.palette().palette,'ocean')
})
test('corrupt and unavailable storage fall back and selection still works for this session', () => {
  for (const raw of ['broken','null','{"palette":"__proto__","mode":"auto"}']) assert.equal(harness({raw}).theme.palette().palette,'lime')
  const h=harness({failStorage:true}); const p=h.theme.set({palette:'berry',mode:'dark'})
  assert.equal(p.palette,'berry'); assert.equal(p.theme,'dark'); assert.equal(p.persisted,false)
})
test('page lifecycle preserves business hooks, updates hidden pages and unsubscribes on unload', () => {
  const h=harness(), calls=[]
  const d=h.register({data:{business:1},onLoad(q){calls.push(['load',q,this]);return 7},onShow(){calls.push(['show'])},onUnload(){calls.push(['unload'])}})
  const page={data:structuredClone(d.data),updates:0,setData(p){Object.assign(this.data,p);this.updates++}}
  h.pages.push(page); assert.equal(d.onLoad.call(page,'query'),7)
  assert.equal(calls[0][2],page); assert.equal(page.data.business,1)
  const other={};h.pages.push(other);const nativeCount=h.native.length
  h.theme.set({palette:'peach',mode:'dark'});assert.match(page.data.appearanceStyle,/#211916/);assert.equal(h.native.length,nativeCount)
  h.pages.pop();d.onShow.call(page);assert.equal(h.native.at(-1),'#211916')
  d.onUnload.call(page);const updates=page.updates;h.theme.set({palette:'ocean'});assert.equal(page.updates,updates)
  assert.deepEqual(calls.map(x=>x[0]),['load','show','unload'])
})
test('icon, tab, sheet and picker update together without leaking listeners', () => {
  const h=harness()
  const icon=h.component('components/icon/index.js',{name:'house',color:'primary'})
  const tab=h.component('custom-tab-bar/index.js'), sheet=h.component('components/sheet/index.js'), picker=h.component('components/theme-picker/index.js')
  h.theme.set({palette:'berry',mode:'dark'})
  assert.match(Buffer.from(icon.data.src.split(',')[1],'base64').toString(),/#F3A6C1/)
  for(const item of [tab,sheet,picker]) assert.match(item.data.appearanceStyle,/#25181F/)
  picker.chooseMode({currentTarget:{dataset:{value:'light'}}});assert.equal(h.theme.name(),'light')
  for(const item of [icon,tab,sheet,picker]) item.detach()
  const counts=[icon,tab,sheet,picker].map(x=>x.updates);h.theme.set({palette:'ocean'})
  assert.deepEqual([icon,tab,sheet,picker].map(x=>x.updates),counts)
})
test('every registered page binds page-meta and the picker escapes transformed/scrolling ancestors', () => {
  const app=JSON.parse(fs.readFileSync(path.join(root,'app.json')))
  for(const page of app.pages) {
    assert.match(fs.readFileSync(path.join(root,page+'.js'),'utf8'),/require\("\.\.\/\.\.\/utils\/theme"\)\.page\(/)
    assert.ok(fs.readFileSync(path.join(root,page+'.wxml'),'utf8').startsWith('<page-meta page-style="{{appearanceStyle}}" />'))
  }
  assert.match(fs.readFileSync(path.join(root,'components/theme-picker/index.wxml'),'utf8'),/<root-portal>[\s\S]*<sheet /)
})
