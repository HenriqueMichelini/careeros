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
const cells=[['en','profile'],['en','account'],['pt-BR','profile'],['pt-BR','account'],['en','late-export'],['en','status-transition'],['en','foreign-after-unknown']]
for(const [locale,kind] of cells){
 const apiPort=await port(),api=`http://127.0.0.1:${apiPort}`,token=randomUUID(),evidence=mkdtempSync(join(tmpdir(),'careeros-deletion-browser-'))
 let server,exit
 try{
  await withAccountBrowser(async({call,evaluate:rawEvaluate})=>{
   const evaluate=async(exp)=>{for(let n=0;;n++){try{return await rawEvaluate(exp)}catch(e){if(n>100||!/(Execution context|Inspected target|Cannot find context)/.test(String(e)))throw e;await pause(25)}}}
   const wait=async(exp)=>{for(let n=0;n<400;n++){if(await evaluate(`Boolean(${exp})`))return;await pause(25)}throw new Error('Timed out: '+exp)}
   const control=name=>evaluate(`fetch('/api/pilot/deletion-test/'+${JSON.stringify(name)}+'?token='+encodeURIComponent(deletionToken),{method:'POST'}).then(async r=>{if(!r.ok)throw new Error('private control failed');return ${JSON.stringify(name)}==='snapshot'?r.json():null})`)
   const press=async(selector)=>{await call('Page.bringToFront');await pause(100);await wait(`document.querySelector(${JSON.stringify(selector)})&&!document.querySelector(${JSON.stringify(selector)}).disabled`);await evaluate(`document.querySelector(${JSON.stringify(selector)}).focus()`);assert.equal(await evaluate(`document.activeElement===document.querySelector(${JSON.stringify(selector)})`),true);await call('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13,text:'\r'});await call('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13})}
   await wait(`document.querySelector('header nav')&&document.querySelector('[data-delete-profile]')`)
   const before=await control('snapshot');assert.equal(before.posts,0);assert.equal(before.primary,1)
   const directDelete=operation=>evaluate(`(async()=>{const owner=deletionWire.session.account.id;const doc=await fetch('/api/profile/current',{headers:{'X-CareerOS-Account':owner}}).then(r=>r.json());const csrf=decodeURIComponent(document.cookie.split('; ').find(x=>x.startsWith('__Host-careeros_csrf=')).split('=')[1]);return fetch('/api/deletion/operations',{method:'POST',headers:{'Content-Type':'application/json','X-CareerOS-Account':owner,'X-CSRF-Token':csrf},body:JSON.stringify({operationId:${JSON.stringify(operation)},kind:${JSON.stringify(kind==='late-export'?'profile':'account')},profileId:${kind==='late-export'?'doc.document.id':"''"},revision:${kind==='late-export'?'doc.document.revision':'0'},confirm:true})}).then(r=>r.status)})()`)
   if(kind==='late-export'){
    await evaluate(`[...document.querySelectorAll('header nav button')].find(b=>b.textContent.trim().toUpperCase()==='PROFILE').click()`)
    await wait(`document.querySelector('[data-profile-export-download]')`)
    await evaluate(`window.holdExport=true;window.createdDownloads=0;const old=URL.createObjectURL.bind(URL);URL.createObjectURL=(...args)=>{window.createdDownloads++;return old(...args)}`)
    await press('[data-profile-export-download]');await wait(`typeof window.releaseExport==='function'`)
    assert.equal(await directDelete('late-export-fence'),202)
    await evaluate(`window.dispatchEvent(new CustomEvent('careeros-deletion-fence',{detail:{accountId:deletionWire.session.account.id}}));window.releaseExport()`)
    await pause(250);assert.equal(await evaluate('window.createdDownloads'),0,'late same-owner deleted Profile downloaded')
    console.log(JSON.stringify({status:'passed',locale,kind,evidence,checks:['actual native export response held before delivery','real confirmed Profile fence without account switch','same-owner lifecycle signal aborts pending download','zero created download URLs/zero inference']}));return
   }
   if(kind==='status-transition'){
    assert.equal(await directDelete('status-transition'),202)
    await control('reauth');await evaluate(`deletionWire.calls=[];window.dispatchEvent(new Event('focus'))`)
    await wait(`document.querySelector('[data-account-deletion]')&&!document.querySelector('[data-close-account]')&&!document.querySelector('header nav')`)
    assert.equal(await evaluate(`deletionWire.calls.some(x=>x.path==='/api/profile/current')`),false,'closed-status transition tried ordinary Profile authority')
    await press('[data-deletion-status]');await wait(`document.querySelector('[data-deletion-result]')`)
    console.log(JSON.stringify({status:'passed',locale,kind,evidence,checks:['same-owner fresh status-only cookie while workspace mounted','no storage signal/reload','workspace disposed before any ordinary Profile call','fresh metadata GET/zero inference']}));return
   }
   if(kind==='foreign-after-unknown'){
    await press('[data-close-account]');await wait(`document.querySelector('[data-confirm-deletion]')`);await evaluate('window.dropDeletionResponse=true');await press('[data-confirm-deletion]')
    await wait(`document.querySelector('[data-account-deletion] [role=alert]')`)
    const oldOwner=await evaluate('deletionWire.session.account.id')
    await control('switch');await evaluate(`window.dispatchEvent(new Event('focus'))`)
    await wait(`deletionWire.session.account?.id!==${JSON.stringify(oldOwner)}&&document.querySelector('header nav')&&!document.querySelector('header nav').closest('[hidden]')`)
    console.log(JSON.stringify({status:'passed',locale,kind,evidence,checks:['lost closure ACK withholds current career workspace','fresh different-owner sign-in clears only obsolete UI uncertainty','independent account remains usable','zero inference']}));return
   }
   await press(kind==='profile'?'[data-delete-profile]':'[data-close-account]')
   await wait(`document.querySelector('[data-confirm-deletion]')`)
   assert.equal((await control('snapshot')).operations,0,'preparation is not confirmation')
   const text=await evaluate(`document.querySelector('[data-account-deletion]').textContent`)
   assert.ok(text.includes(locale==='en'?'Export is optional':'Exportar é opcional'))
   if(kind==='profile')await evaluate('window.dropDeletionResponse=true')
   await press('[data-confirm-deletion]')
   if(kind==='account'){
    await wait(`document.querySelector('a[href="/api/auth/google/start"]')`)
    assert.equal(await evaluate(`Boolean(document.querySelector('header nav'))`),false)
    await control('reauth');await evaluate(`location.reload()`)
   }
   await wait(`document.querySelector('[data-deletion-result]')`)
   const after=await control('snapshot');assert.equal(after.posts,1);assert.equal(after.operations,1);assert.equal(after.primary,0)
   assert.equal(after.accountState,kind==='account'?'deleted':'active')
   assert.ok((await evaluate(`document.querySelector('[data-deletion-result]').textContent`)).includes(locale==='en'?'removal not verified':'remoção não verificada'))
   if(kind==='account'){
    assert.equal(await evaluate(`Boolean(document.querySelector('header nav'))`),false)
    assert.equal(await evaluate(`fetch('/api/profile/current',{headers:{'X-CareerOS-Account':deletionWire.session.account.id}}).then(r=>r.status)`),401)
   }else{
    await wait(`document.querySelector('header nav')`)
    assert.equal(await evaluate(`fetch('/api/profile/export',{headers:{'X-CareerOS-Account':deletionWire.session.account.id}}).then(r=>r.status)`),404)
   }
   await control('purge');await press('[data-deletion-status]')
   await wait(`document.querySelector('[data-deletion-result]')?.textContent.includes(${JSON.stringify(locale==='en'?'Active content removed.':'Conteúdo ativo removido.')})`)
   assert.equal((await control('snapshot')).posts,1,'status/purge/reload must not repeat deletion POST')
   assert.equal(await evaluate(`deletionWire.calls.some(x=>['qualification','/cv/generate','/api/drafts'].some(p=>x.path.includes(p))&&x.method==='POST')`),false)
   console.log(JSON.stringify({status:'passed',locale,kind,evidence,checks:['actual React App/Vite/authenticated Go/private PG','trusted keyboard explicit bound confirmation','no export prerequisite or automatic mutation',kind==='profile'?'lost Profile response status-only recovery':'closed-account fresh status-only sign-in/no workspace','fenced pending versus primary removal versus unverified residual copies','one deletion operation/zero inference']}))
  },{path:'/tests/account-deletion-harness.html',env:{VITE_CAREEROS_ACCOUNT_MODE:'1',CAREEROS_API_TARGET:api},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(locale)});window.deletionToken=${JSON.stringify(token)}`,beforeNavigate:async({port})=>{
   const config=join(evidence,'config.json');writeFileSync(config,JSON.stringify({Origin:`http://127.0.0.1:${port}`,Address:`127.0.0.1:${apiPort}`,Token:token}),{mode:0o600})
   const log=createWriteStream(join(evidence,'server.jsonl'),{flags:'wx'})
   server=spawn('rtk',['proxy','go','test','-json','-vet=off','-count=1','-run','^TestDeletionBrowserServer$','-timeout=180s','./internal/accountapi'],{cwd:root,env:{...process.env,CAREEROS_DELETION_BROWSER_CONFIG:config,GOCACHE:'/tmp/careeros-go-cache'},stdio:['ignore','pipe','pipe']});server.stdout.pipe(log);server.stderr.pipe(log);exit=new Promise(resolve=>server.on('exit',code=>{log.end();resolve(code)}))
   for(let n=0;n<200;n++){try{if((await fetch(`${api}/api/pilot/deletion-test/health?token=${token}`)).ok)return}catch{};if(server.exitCode!==null)throw new Error('Private server exited: '+evidence);await pause(100)}throw new Error('Private server timeout: '+evidence)
  }})
 }finally{if(server){try{await fetch(`${api}/api/pilot/deletion-test/stop?token=${token}`,{method:'POST'})}catch{};const code=await Promise.race([exit,pause(10000).then(()=>{server.kill('SIGTERM');return 'terminated'})]);assert.equal(code,0,'server terminal status')}}
}
