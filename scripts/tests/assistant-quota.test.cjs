const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

function harness() {
  const quota = { limit:20, used:0, remaining:20, reset_at:'2026-09-28T00:00:00+08:00' }
  const state = { quota, requests:[], notices:[], timers:new Map(), nextTimer:0 }
  let definition
  const api = { token:()=>'local-test', get:async url=>url.endsWith('/status')?{llm_enabled:false,suggestions:[],quota:state.quota}:[] }
  const wx = { request:options=>{state.requests.push(options);return {abort(){},onChunkReceived(fn){state.chunk=fn}}} }
  const dependencies = { api, session:{requireLogin:()=>true}, ui:{toast:value=>state.notices.push(value),haptic(){}}, media:{assetUrl:x=>x}, format:{relativeDate:x=>x} }
  vm.runInNewContext(fs.readFileSync(path.resolve(__dirname,'../../miniprogram/pages/chat/chat.js'),'utf8'),{
    Page:value=>{definition=value},require:name=>dependencies[name.split('/').at(-1)],wx,
    getApp:()=>({globalData:{apiBase:'https://local.invalid/api'},navMetrics:()=>({navHeight:80})}),
    setTimeout:fn=>{state.timers.set(++state.nextTimer,fn);return state.nextTimer},clearTimeout:id=>state.timers.delete(id),
  })
  const page = { data:structuredClone(definition.data),setData(patch,callback){
    for(const [key,value] of Object.entries(patch)){
      const keys=key.replace(/\[(\d+)\]/g,'.$1').split('.');let target=this.data
      for(const segment of keys.slice(0,-1))target=target[segment]
      target[keys.at(-1)]=value
    }
    callback?.()
  }}
  for(const [key,value] of Object.entries(definition))if(typeof value==='function')page[key]=value.bind(page)
  return {page,api,state}
}
function bytes(text) { return Uint8Array.from(Buffer.from(text,'utf8')).buffer }

test('mini assistant blocks send until quota loads and preserves input when exhausted',async()=>{
  const {page,state}=harness()
  page.send('今晚吃什么')
  assert.equal(state.requests.length,0);assert.equal(page.data.draft,'今晚吃什么')
  await page.loadStatus()
  page.applyQuota({...state.quota,used:20,remaining:0})
  page.sendDraft()
  assert.equal(state.requests.length,0);assert.equal(page.data.draft,'今晚吃什么')
  state.quota={...state.quota,limit:0,remaining:0}
  await page.loadStatus();assert.equal(page.data.canSend,false)
  assert.match(page.data.quotaText,/暂停/)
})

test('mini assistant handles chunked quota rejection without losing the unsent message',async()=>{
  const {page,state}=harness();await page.loadStatus()
  page.send('用鸡蛋做什么')
  assert.equal(state.requests.length,1)
  state.quota={...state.quota,used:20,remaining:0}
  const body=JSON.stringify({code:42901,message:'今天次数已用完',data:{quota:state.quota}})
  state.chunk({data:bytes(body)})
  const request=state.requests[0]
  request.success({statusCode:429,data:new ArrayBuffer(0)});request.complete()
  await Promise.resolve();await Promise.resolve()
  assert.equal(page.data.messages.length,0)
  assert.equal(page.data.draft,'用鸡蛋做什么')
  assert.equal(page.data.canSend,false)
  assert.equal(page.data.busy,false)
  assert.match(state.notices.join(' '),/次数已用完/)
})

test('accepted SSE quota beats an older status response and unload clears refresh timers',async()=>{
  const {page,api,state}=harness();await page.loadStatus();page._visible=true
  let resolve
  api.get=()=>new Promise(yes=>{resolve=yes})
  const stale=page.loadStatus()
  page.handleFrame('event: quota\ndata: '+JSON.stringify({...state.quota,used:20,remaining:0}))
  resolve({llm_enabled:false,suggestions:[],quota:state.quota});await stale
  assert.equal(page.data.quota.remaining,0)
  assert.equal(page.data.canSend,false)
  assert.ok(state.timers.size)
  page.onUnload();assert.equal(state.timers.size,0)
})

test('site-wide pause disables sending even when personal quota remains', async () => {
  const { page, state } = harness()
  state.quota = { ...state.quota, blocked_reason: 'site_limit' }
  await page.loadStatus()
  page.send('晚餐想吃番茄炒蛋')
  assert.equal(page.data.canSend, false)
  assert.equal(state.requests.length, 0)
  assert.equal(page.data.draft, '晚餐想吃番茄炒蛋')
  assert.match(page.data.quotaText, /已用完|暂停/)
})

test('review progress leaves no reply until the approved text and cards arrive', async () => {
  const { page, state } = harness()
  await page.loadStatus()
  page.send('推荐一道晚餐')
  const frame = (event, data) => page.handleFrame(`event: ${event}\ndata: ${JSON.stringify(data)}`)
  frame('status', { message: '正在核对食谱内容…' })
  frame('tool_start', { id: 'tool-1', name: 'recommend_dishes', label: '按口味挑菜' })
  frame('tool_end', { id: 'tool-1', name: 'recommend_dishes', ok: true })
  const reply = () => page.data.messages[page._assistantIndex]
  assert.equal(reply().content, '')
  assert.equal(reply().cards.length, 0)
  assert.match(page.data.progressText, /核对/)
  frame('delta', { text: '可以试试番茄炒蛋。' })
  frame('cards', { cards: [{ type: 'dishes', title: '为你推荐', items: [{ dish: { id: 1, name: '番茄炒蛋' }, reasons: ['做起来简单'] }] }] })
  frame('done', { session_id: 1, message_id: 2 })
  state.requests[0].complete()
  assert.equal(reply().content, '可以试试番茄炒蛋。')
  assert.equal(reply().cards[0].items[0].name, '番茄炒蛋')
  assert.equal(reply().streaming, false)
})
