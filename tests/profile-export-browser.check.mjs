import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {mkdtempSync,writeFileSync,createWriteStream,readFileSync,existsSync,mkdirSync,readdirSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import {createServer} from 'node:net'
import {randomUUID,createHash} from 'node:crypto'
import {withAccountBrowser,pause} from './account-browser.fixture.mjs'
const root=new URL('..',import.meta.url).pathname
const expected='{"format":"careeros-profile-export","version":1,"profile":'+readFileSync(join(root,'internal/accountapi/testdata/profile-export-v1.json'),'utf8')+'}'
const compact=value=>JSON.stringify(JSON.parse(value))
async function port(){return new Promise(resolve=>{const s=createServer();s.listen(0,'127.0.0.1',()=>{const n=s.address().port;s.close(()=>resolve(n))})})}
for(const locale of ['en','pt-BR']){
 const apiPort=await port(),api=`http://127.0.0.1:${apiPort}`,token=randomUUID(),evidence=mkdtempSync(join(tmpdir(),'careeros-profile-export-browser-'))
 const downloads=join(evidence,'downloads');mkdirSync(downloads)
 let server,exit
 try{
  await withAccountBrowser(async({call,evaluate:rawEvaluate})=>{
   const evaluate=async(exp)=>{for(let n=0;;n++){try{return await rawEvaluate(exp)}catch(e){if(n>100||!/(Execution context|Inspected target|Cannot find context)/.test(String(e)))throw e;await pause(25)}}}
   const wait=async(exp)=>{for(let n=0;n<400;n++){if(await evaluate(`Boolean(${exp})`))return;await pause(25)}throw new Error('Timed out: '+exp)}
   const control=name=>evaluate(`fetch('/api/pilot/export-test/'+${JSON.stringify(name)}+'?token='+encodeURIComponent(exportToken),{method:'POST'}).then(r=>{if(!r.ok)throw new Error('private control failed')})`)
   await wait(`document.querySelector('header nav')`)
   await evaluate(`[...document.querySelectorAll('header nav button')].find(b=>/^(PROFILE|PERFIL)$/i.test(b.textContent.trim())).click()`)
   await wait(`document.querySelector('[data-profile-export-download]')`)
   await call('Browser.setDownloadBehavior',{behavior:'allow',downloadPath:downloads,eventsEnabled:true})
   const press=async()=>{
    await call('Page.bringToFront');await pause(100);await wait(`document.querySelector('[data-profile-export-download]')?.getClientRects().length>0 && !document.querySelector('[data-profile-export-download]').disabled`);await evaluate(`document.querySelector('[data-profile-export-download]').focus()`)
    assert.equal(await evaluate(`document.activeElement===document.querySelector('[data-profile-export-download]')`),true)
    await call('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13,text:'\r'})
    await call('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13})
   }
   assert.equal(await evaluate(`exportWire.statuses.length`),0,'no automatic export')
   await press();await wait(`document.querySelector('[data-profile-export-message]')?.getAttribute('role')==='status'`)
   const file=join(downloads,'careeros-profile-v1.json')
   for(let n=0;n<100&&!existsSync(file);n++)await pause(50)
   assert.equal(existsSync(file),true,'real attachment file missing')
   const bytes=readFileSync(file)
   assert.equal(compact(bytes.toString()),compact(expected))
   // Exact bytes include opaque numeric lexemes, independent of JS parsed values.
   assert.ok(bytes.includes(Buffer.from('9007199254740993')));assert.ok(bytes.includes(Buffer.from('1e+308')));assert.ok(bytes.includes(Buffer.from('5e-324')))
   assert.ok((await evaluate(`document.querySelector('[data-profile-export-message]').textContent`)).includes(locale==='en'?'Download started':'Download iniciado'))
   await control('window');await press();await wait(`exportWire.statuses.length===2&&!document.querySelector('[data-profile-export-download]').disabled`)
   assert.deepEqual(await evaluate('exportWire.statuses'),[200,200])
   await control('expire');await press();await wait(`document.querySelector('[data-profile-export-message]')?.getAttribute('role')==='alert'`)
   assert.ok((await evaluate(`document.querySelector('[data-profile-export-message]').textContent`)).includes(locale==='en'?'Sign in again':'Entre novamente'))
   await control('reauth');await press();await wait(`exportWire.statuses.length===4&&!document.querySelector('[data-profile-export-download]').disabled`)
   await control('revoke');await press();await wait(`exportWire.statuses.length===5&&!document.querySelector('[data-profile-export-download]').disabled`)
   assert.ok((await evaluate(`document.querySelector('[data-profile-export-message]').textContent`)).includes(locale==='en'?'current access':'acesso atual'))
   assert.deepEqual(await evaluate('exportWire.statuses'),[200,200,401,200,403])
   assert.deepEqual(await evaluate('exportWire.methods'),['GET','GET','GET','GET','GET'])
   assert.equal(await evaluate(`Object.keys(sessionStorage).some(k=>k.includes('profile-export'))`),false)
   await control('restore')
   const beforeLate=readdirSync(downloads).filter(f=>f.endsWith('.json')).length
   const previousOwner=await evaluate('exportWire.owner')
   await evaluate('window.holdExport=true');await press();await wait(`typeof window.releaseExport==='function'`)
   await control('switch');await evaluate(`window.dispatchEvent(new Event('focus'))`)
   await wait(`exportWire.owner!==${JSON.stringify(previousOwner)} && document.querySelector('header nav') && document.querySelector('[data-profile-export-download]') && !document.querySelector('[data-profile-export-download]').disabled`)
   await pause(100);await evaluate('window.releaseExport()');await pause(200)
   assert.equal(readdirSync(downloads).filter(f=>f.endsWith('.json')).length,beforeLate,'late foreign-owner reply initiated a download')
   assert.equal(await evaluate(`document.querySelector('[data-profile-export-message]')`),null)
   console.log(JSON.stringify({status:'passed',locale,evidence,downloadBytes:bytes.length,downloadSHA256:createHash('sha256').update(bytes).digest('hex'),checks:['actual React App/Vite/authenticated Go/PilotRetrieve/private PG','trusted keyboard real JSON attachment','independent full retained values/opaque numeric bytes','no automatic export or Profile write','post-pilot window200/session401/reauth200/revoked403','EN/PT truthful metadata-only errors','late prior-owner response discarded after real account switch']}))
  },{path:'/tests/profile-export-harness.html',env:{VITE_CAREEROS_ACCOUNT_MODE:'1',CAREEROS_API_TARGET:api},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(locale)});window.exportToken=${JSON.stringify(token)}`,beforeNavigate:async({port})=>{
   const config=join(evidence,'config.json');writeFileSync(config,JSON.stringify({Origin:`http://127.0.0.1:${port}`,Address:`127.0.0.1:${apiPort}`,Token:token}),{mode:0o600})
   const log=createWriteStream(join(evidence,'server.jsonl'),{flags:'wx'})
   server=spawn('rtk',['proxy','go','test','-json','-vet=off','-count=1','-run','^TestProfileExportBrowserServer$','-timeout=180s','./internal/accountapi'],{cwd:root,env:{...process.env,CAREEROS_EXPORT_BROWSER_CONFIG:config,GOCACHE:'/tmp/careeros-go-cache'},stdio:['ignore','pipe','pipe']});server.stdout.pipe(log);server.stderr.pipe(log);exit=new Promise(resolve=>server.on('exit',code=>{log.end();resolve(code)}))
   for(let n=0;n<200;n++){try{if((await fetch(`${api}/api/pilot/export-test/health?token=${token}`)).ok)return}catch{};if(server.exitCode!==null)throw new Error('Private server exited: '+evidence);await pause(100)}
   throw new Error('Private server timeout: '+evidence)
  }})
 }finally{
  if(server){try{await fetch(`${api}/api/pilot/export-test/stop?token=${token}`,{method:'POST'})}catch{};const code=await Promise.race([exit,pause(10000).then(()=>{server.kill('SIGTERM');return 'terminated'})]);assert.equal(code,0,'server terminal status')}
 }
}
