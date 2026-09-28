const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {createRequire}=require('node:module');
const root=process.cwd(),fr=createRequire(path.join(root,'frontend/package.json')),ts=fr('typescript'),{JSDOM}=fr('jsdom');
const values=new Map();const shared={getItem:k=>values.get(k)||null,setItem:(k,v)=>values.set(k,String(v)),removeItem:k=>values.delete(k)};
function tab(url='https://cook.arrebyte.top/') {
 const dom=new JSDOM('',{url}),win=dom.window;
 function load(file,stubs={}) {
  const module={exports:{}};
  const code=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;
  new Function('require','module','exports','window','localStorage','sessionStorage','navigator',code)(n=>n in stubs?stubs[n]:fr(n),module,module.exports,win,shared,win.sessionStorage,win.navigator);
  return module.exports;
 }
 const client=load('frontend/src/api/client.ts');let sent;
 client.default.defaults.adapter=async config=>{sent=config.headers.Authorization;return {data:{code:0,data:{id:sent==='Bearer token-A'?1:2}},status:200,statusText:'OK',config,headers:{}}};
 const mini=load('frontend/src/lib/miniprogram.ts');
 const store=load('frontend/src/store/useAuthStore.ts',{'@/api':{meApi:{get:()=>client.api('GET','/me')}},'@/api/client':client,'@/lib/miniprogram':mini}).useAuthStore;
 return {win,client,mini,store,sent:()=>sent};
}
(async()=>{
 const a=tab(),b=tab();a.store.getState().setSession('token-A',{id:1});b.store.getState().setSession('token-B',{id:2});
 a.win.dispatchEvent(new a.win.StorageEvent('storage',{key:'ninimenu_token',oldValue:'token-A',newValue:'token-B'}));
 await a.client.api('POST','/dishes',{name:'A private draft'});
 assert.equal(a.store.getState().user.id,1);assert.equal(a.sent(),'Bearer token-B');
 console.log('PASS: tab A displays user 1; after real storage event its mutation uses user 2 token');
 values.clear();const victim=tab('https://cook.arrebyte.top/#token=token-B');victim.store.getState().setSession('token-A',{id:1});
 assert.equal(victim.mini.isMiniProgram(),false);await victim.store.getState().bootstrap();
 assert.equal(victim.store.getState().user.id,2);assert.equal(victim.client.getToken(),'token-B');
 console.log('PASS: ordinary browser URL fragment replaces existing user 1 session with supplied user 2 token');
})().catch(e=>{console.error(e);process.exitCode=1});
