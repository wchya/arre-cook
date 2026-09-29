// Run against an isolated Go server/database, never production. No API response fixtures.
// ARRE_ACCEPTANCE_URL=http://127.0.0.1:8080 ARRE_ACCEPTANCE_ACCOUNT=... ARRE_ACCEPTANCE_PASSWORD=... node scripts/acceptance/mini-real-backend.cjs
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const assert = require('node:assert/strict')
const base = process.env.ARRE_ACCEPTANCE_URL
assert.ok(base && ['127.0.0.1','localhost'].includes(new URL(base).hostname), 'Use an isolated local Go backend')
assert.ok(process.env.ARRE_ACCEPTANCE_ACCOUNT && process.env.ARRE_ACCEPTANCE_PASSWORD, 'Dedicated acceptance credentials required')
const root = path.resolve(__dirname,'../../miniprogram')
const cache = new Map(), storage = new Map(), pending = new Set(), requests = [], pages = [], checks = []
let definition
const app = {globalData:{apiBase:base+'/api',origin:base,user:null},navMetrics:()=>({navHeight:88,statusBarHeight:24,navBarHeight:44,windowWidth:390,windowHeight:844})}
const wx = {
  getStorageSync:key=>storage.get(key),setStorageSync:(k,v)=>storage.set(k,v),removeStorageSync:k=>storage.delete(k),
  getAppBaseInfo:()=>({theme:'light'}),getWindowInfo:()=>({windowWidth:390,windowHeight:844,pixelRatio:2}),getDeviceInfo:()=>({platform:'devtools'}),
  onThemeChange() {}, setBackgroundColor() {}, setBackgroundTextStyle() {}, setNavigationBarColor() {}, setNavigationBarTitle() {},
  showToast() {},vibrateShort() {},switchTab() {},reLaunch() {},navigateTo() {},navigateBack() {},hideKeyboard() {},
  showModal:o=>o.success({confirm:true}),requirePrivacyAuthorize:o=>o.success(),
  request(o) {
    const task = fetch(o.url,{method:o.method,headers:o.header,body:o.method==='GET'?undefined:JSON.stringify(o.data),signal:AbortSignal.timeout(20000)})
      .then(async r=>{requests.push({method:o.method,path:new URL(o.url).pathname,status:r.status});o.success({statusCode:r.status,data:await r.json()})})
      .catch(e=>o.fail(e)).finally(()=>pending.delete(task))
    pending.add(task); return {abort() {}}
  },
}
function load(file) {
  file=path.resolve(file);if(!file.endsWith('.js'))file+='.js'
  if(cache.has(file))return cache.get(file).exports
  const module={exports:{}};cache.set(file,module)
  vm.runInNewContext(fs.readFileSync(file,'utf8'),{module,exports:module.exports,wx,getApp:()=>app,getCurrentPages:()=>pages,
    Page:d=>{definition=d},require:name=>load(path.resolve(path.dirname(file),name)),console,setTimeout,clearTimeout,setInterval,clearInterval,URL,TextDecoder,Uint8Array,ArrayBuffer,
  },{filename:path.relative(root,file)})
  return module.exports
}
async function settle() {
  for(let i=0;i<30;i++) {await Promise.all([...pending]);await new Promise(r=>setTimeout(r,20));if(!pending.size)return}
  throw Error('Requests did not settle')
}
async function page(name,query={}) {
  load(path.join(root,'pages',name,name+'.js'))
  const d=definition,p={data:structuredClone(d.data),route:'pages/'+name+'/'+name,selectComponent:()=>null,getTabBar:()=>null,
    setData(patch,cb) {for(const [key,value] of Object.entries(patch)){const keys=key.replace(/\[(\d+)\]/g,'.$1').split('.');let obj=this.data;for(const k of keys.slice(0,-1))obj=obj[k]??=(/^[0-9]+$/.test(k)?[]:{});obj[keys.at(-1)]=value}cb?.()},
  }
  for(const [k,v]of Object.entries(d))if(typeof v==='function')p[k]=v.bind(p)
  pages.push(p);p.onLoad?.(query);await settle();return p
}
const event = values => ({currentTarget:{dataset:values},detail:{value:values.value}})
;(async()=>{
  const api=load(path.join(root,'utils/api')),theme=load(path.join(root,'utils/theme'))
  const login=await page('login')
  login.setData({account:process.env.ARRE_ACCEPTANCE_ACCOUNT,password:process.env.ARRE_ACCEPTANCE_PASSWORD,agreed:true})
  await login.runWithConsent('doSubmitPassword');await settle();assert.ok(api.token(),'native login handler must save real session')
  checks.push('password-login-and-consent')
  const home=await page('home');assert.ok(home.data.current?.id,'real recommendations loaded')
  for(const palette of ['lime','peach','ocean','berry'])for(const mode of ['light','dark']) {theme.set({palette,mode});assert.ok(home.data.appearanceStyle.includes(theme.palette().bg))}
  checks.push('eight-themes-with-real-recommendations')
  await home.pickMeal(event({meal:'lunch'}));const initial=home.data.current.id
  await home.changeRecommend();await settle();assert.notEqual(home.data.current.id,initial)
  await home.recordCurrent(event({meal:'lunch'}));await settle();await home.recordCurrent(event({meal:'dinner'}));await settle()
  const recordData=await api.get('/records',{pageSize:50});assert.ok(recordData.items.some(x=>x.meal_type==='lunch'));assert.ok(recordData.items.some(x=>x.meal_type==='dinner'))
  const count=recordData.total;await home.recordCurrent(event({meal:'lunch'}));await settle();assert.equal((await api.get('/records',{pageSize:50})).total,count)
  checks.push('meal-pick-change-record-lunch-dinner-and-duplicate-guard')
  const detail=await page('dish',{id:String(home.data.current.id)});assert.ok(detail.data.dish?.name, detail.data.loadError)
  const favorite=detail.data.favorite;await detail.toggleFavorite();assert.notEqual(detail.data.favorite,favorite);await detail.toggleFavorite();assert.equal(detail.data.favorite,favorite)
  checks.push('dish-detail-favorite-and-unfavorite')
  const prefs=await page('preferences');const old={...prefs.data.form};prefs.chooseSpice(event({v:1}));await prefs.save();assert.equal((await api.get('/me/preferences')).spice_level,1);await api.put('/me/preferences',old)
  checks.push('preferences-save-and-readback')
  const history=await page('history');await history.loadAll();assert.ok(history.data.recent.length);assert.equal(history.data.monthError,'')
  const id=recordData.items[0].id;history.openRate(event({id}));history.pickRateMood(event({mood:'yum'}));history.onRateRemark(event({value:'theme acceptance'}));await history.saveRate();await settle();assert.ok(history.data.recent.some(x=>x.id===id && x.mood==='yum' && x.remark==='theme acceptance'))
  checks.push('history-calendar-and-record-rating-readback')
  const me=await page('me');await me.load();assert.ok(me.data.user);assert.ok(me.data.stats)
  checks.push('profile-and-stats')
  for(const endpoint of ['/settings','/favorites','/week-plan','/shopping-list','/family','/food-journal','/health-report','/notifications','/achievements','/profile','/photo-wall','/suggestions','/me/agent-tokens','/assistant/status','/assistant/video-recipe/status']) {await api.get(endpoint);checks.push('GET '+endpoint)}
  const tomorrow=await page('tomorrow');assert.equal(tomorrow.data.loading,false);await tomorrow.save();await settle();assert.equal(tomorrow.data.saving,false)
  checks.push('tomorrow-menu-load-and-save')
  for(const record of (await api.get('/records',{pageSize:100})).items)await api.delete('/records/'+record.id)
  const session=load(path.join(root,'utils/session'));session.logout();assert.equal(api.token(),'');assert.equal(theme.palette().palette,'berry')
  checks.push('cleanup-and-logout-retains-appearance')
  for(const p of pages)p.onUnload?.()
  const failures=requests.filter(x=>x.status>=400);assert.deepEqual(failures,[])
  console.log(JSON.stringify({backend:base,transport:'real HTTP; native runtime adapters only',checks,requestCount:requests.length,failedRequests:failures},null,2))
})().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{for(const p of pages)p.onUnload?.()})
