import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {mkdtempSync,writeFileSync,createWriteStream} from 'node:fs'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import {createServer} from 'node:net'
import {randomUUID} from 'node:crypto'
import {withAccountBrowser,pause} from './account-browser.fixture.mjs'
const root=new URL('..',import.meta.url).pathname
async function port(){return new Promise(resolve=>{const s=createServer();s.listen(0,'127.0.0.1',()=>{const n=s.address().port;s.close(()=>resolve(n))})})}
for(const locale of ['en','pt-BR']){
 const apiPort=await port(),api=`http://127.0.0.1:${apiPort}`,token=randomUUID(),evidence=mkdtempSync(join(tmpdir(),'careeros-control-browser-'))
 let server,exit
 try{await withAccountBrowser(async({call,evaluate})=>{
  const wait=async exp=>{for(let n=0;n<400;n++){try{if(await evaluate(`Boolean(${exp})`))return}catch(e){if(!/(Execution context|Inspected target|Cannot find context)/.test(String(e)))throw e}await pause(25)}throw Error('Timed out: '+exp)}
  const control=name=>evaluate(`fetch('/api/pilot/deletion-test/'+${JSON.stringify(name)}+'?token='+encodeURIComponent(deletionToken),{method:'POST'}).then(async r=>{if(!r.ok)throw Error('private control');return ${JSON.stringify(name)}==='snapshot'?r.json():null})`)
  const press=async selector=>{await call('Page.bringToFront');await pause(100);await wait(`document.querySelector(${JSON.stringify(selector)})&&!document.querySelector(${JSON.stringify(selector)}).disabled`);await evaluate(`document.querySelector(${JSON.stringify(selector)}).focus()`);assert.equal(await evaluate(`document.activeElement===document.querySelector(${JSON.stringify(selector)})`),true);await call('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13,text:'\r'});await call('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13})}
  await wait(`document.querySelector('header nav')&&document.querySelector('[data-delete-profile]')`)
  await control('compromise');await evaluate(`window.dispatchEvent(new Event('focus'))`)
  await wait(`!document.querySelector('header nav')&&document.querySelector('a[href="/api/auth/google/start"]')`)
  assert.equal((await control('snapshot')).posts,0)
  await control('reauth');await evaluate(`window.dispatchEvent(new Event('focus'))`)
  await wait(`deletionWire.session.restrictedRights&&document.querySelector('[data-close-account]')`)
  assert.equal(await evaluate(`Boolean(document.querySelector('header nav')||document.querySelector('[data-delete-profile]')||document.querySelector('[data-profile-export-download]'))`),false)
  const status=await evaluate(`fetch('/api/rights/status',{headers:{'X-CareerOS-Account':deletionWire.session.account.id}}).then(async r=>({status:r.status,body:await r.json()}))`);assert.equal(status.status,200);assert.equal(status.body.state,"restricted");assert.equal(status.body.canRequestDeletion,true)
  for(const path of ['/api/profile/current','/api/profile/export']) assert.ok([401,403].includes(await evaluate(`fetch(${JSON.stringify(path)},{headers:{'X-CareerOS-Account':deletionWire.session.account.id}}).then(r=>r.status)`)))
  await press('[data-close-account]');await wait(`document.querySelector('[data-confirm-deletion]')`)
  assert.equal((await control('snapshot')).posts,0,'preparing restricted request is not closure')
  const text=await evaluate(`document.querySelector('[data-account-deletion]').textContent`);assert.ok(text.includes(locale==='en'?'Career content remains blocked':'conteúdo profissional continua bloqueado'))
  await press('[data-confirm-deletion]');await wait(`!document.querySelector('[data-close-account]')&&document.querySelector('a[href="/api/auth/google/start"]')`)
  const snapshot=await control('snapshot');assert.equal(snapshot.posts,1);assert.equal(snapshot.operations,1);assert.equal(snapshot.primary,0);assert.equal(snapshot.accountState,'deleted')
  assert.equal(await evaluate(`deletionWire.calls.some(c=>c.method==='POST'&&['qualification','/api/drafts','/cv/generate'].some(p=>c.path.includes(p)))`),false)
  console.log(JSON.stringify({status:'passed',locale,evidence,checks:['actual React/Vite/private Go/PostgreSQL','mounted ordinary workspace withheld after committed compromise','fresh verified identity restricted to metadata and explicit rights request','no Profile/export/activation/ordinary workspace','keyboard deliberate account closure once/zero provider sends']}))
 },{path:'/tests/account-deletion-harness.html',env:{VITE_CAREEROS_ACCOUNT_MODE:'1',CAREEROS_API_TARGET:api},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(locale)});window.deletionToken=${JSON.stringify(token)}`,beforeNavigate:async({port})=>{
  const config=join(evidence,'config.json');writeFileSync(config,JSON.stringify({Origin:`http://127.0.0.1:${port}`,Address:`127.0.0.1:${apiPort}`,Token:token}),{mode:0o600})
  const log=createWriteStream(join(evidence,'server.jsonl'),{flags:'wx'})
  server=spawn('rtk',['proxy','go','test','-json','-vet=off','-count=1','-run','^TestDeletionBrowserServer$','-timeout=180s','./internal/accountapi'],{cwd:root,env:{...process.env,CAREEROS_DELETION_BROWSER_CONFIG:config,GOCACHE:'/tmp/careeros-go-cache'},stdio:['ignore','pipe','pipe']});server.stdout.pipe(log);server.stderr.pipe(log);exit=new Promise(resolve=>server.on('exit',code=>{log.end();resolve(code)}))
  for(let n=0;n<200;n++){try{if((await fetch(`${api}/api/pilot/deletion-test/health?token=${token}`)).ok)return}catch{};if(server.exitCode!==null)throw Error('Private server exited: '+evidence);await pause(100)}throw Error('Private server timeout: '+evidence)
 }})}finally{if(server){try{await fetch(`${api}/api/pilot/deletion-test/stop?token=${token}`,{method:'POST'})}catch{};const code=await Promise.race([exit,pause(10000).then(()=>{server.kill('SIGTERM');return 'terminated'})]);assert.equal(code,0,'server terminal status')}}
}
