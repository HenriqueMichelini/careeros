import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {mkdtempSync,writeFileSync,createWriteStream} from 'node:fs'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import {createServer} from 'node:net'
import {randomUUID} from 'node:crypto'
import {withAccountBrowser,pause} from './account-browser.fixture.mjs'

const root=new URL('..',import.meta.url).pathname
async function freePort(){return new Promise(resolve=>{const server=createServer();server.listen(0,'127.0.0.1',()=>{const port=server.address().port;server.close(()=>resolve(port))})})}
for(const locale of (process.env.CAREEROS_DRAFT_BROWSER_LOCALE ? [process.env.CAREEROS_DRAFT_BROWSER_LOCALE] : ['en','pt-BR'])){
 const apiPort=await freePort(),token=randomUUID(),api=`http://127.0.0.1:${apiPort}`,evidence=mkdtempSync(join(tmpdir(),'careeros-281-real-browser-'))
 let server,serverExit
 try{
  await withAccountBrowser(async({evaluate:rawEvaluate,call})=>{
   // CDP may briefly reject evaluation while the requested new document commits.
   // Retry that transport transition only; never swallow a page/assertion error.
   const evaluate=async(expression)=>{for(let n=0;;n++){try{return await rawEvaluate(expression)}catch(error){if(n>=100||!/(Inspected target navigated or closed|Execution context was destroyed|Cannot find context)/.test(String(error)))throw error;await pause(25)}}}
   const wait=async(expression)=>{for(let n=0;n<400;n++){if(await evaluate(`Boolean(${expression})`))return;await pause(25)}throw new Error('Timed out: '+expression)}
   const control=async(name,method='POST')=>evaluate(`fetch('/api/drafts/test/${name}?token='+encodeURIComponent(draftControlToken),{method:${JSON.stringify(method)}}).then(async r=>{if(!r.ok)throw new Error('fixture control failed');return r.status===204?null:r.json()})`)
   await wait(`document.querySelector('[data-draft-workspace]')`)
   assert.deepEqual(await control('stats','GET'),{generation:0,completion:0,allowanceEffects:0})
   await evaluate(`(()=>{const input=document.querySelector('[data-draft-job]');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(input,'Java required.');input.dispatchEvent(new Event('input',{bubbles:true}))})()`)
   await wait(`!document.querySelector('[data-draft-generate]').disabled`)
   await evaluate(`document.querySelector('[data-draft-generate]').click()`)
   await wait(`document.querySelector('[role="alert"]') && draftWire.responses.some(r=>r.method==='POST'&&r.status===201)`)
   const initial=await evaluate(`draftWire.responses.find(r=>r.method==='POST'&&r.status===201).body`)
   assert.equal(initial.state,'ready');assert.equal(initial.artifactVersion,1);assert.equal(initial.bundle.binding.language,locale)
   assert.equal(initial.bundle.identity.fullName,'Original Person');assert.ok(initial.bundle.coverLetter.endsWith('\nOriginal Person'))
   assert.equal(initial.bundle.employerAnswers.state,'none_requested')
   const posts=await evaluate(`draftWire.requests.filter(r=>r.path==='/api/drafts'&&r.method==='POST')`)
   assert.equal(posts.length,1);assert.equal(posts[0].callerKey,false);assert.equal(posts[0].body.profileSource.revision,2)
   assert.ok(!('repository'in posts[0].body));assert.ok(!('profileEvidence'in posts[0].body))
   assert.deepEqual(await control('stats','GET'),{generation:1,completion:1,allowanceEffects:0})
   const oldDocument=await evaluate(`draftWire.documentID`)
   await call('Page.reload',{ignoreCache:true})
   await wait(`window.draftWire?.documentID!==${JSON.stringify(oldDocument)} && document.querySelector('[data-draft-result]')`)
   const recovered=await evaluate(`draftWire.responses.find(r=>r.method==='GET'&&r.body.state==='ready').body`)
   assert.deepEqual(recovered,initial)
   assert.equal(await evaluate(`document.querySelector('[data-draft-result]').getAttribute('data-draft-result')`),initial.action.id)
   assert.equal(await evaluate(`document.querySelectorAll('[data-draft-artifact]').length`),4)
   assert.equal(await evaluate(`draftWire.requests.filter(r=>r.path==='/api/drafts'&&r.method==='POST').length`),0)
   assert.deepEqual(await control('stats','GET'),{generation:1,completion:1,allowanceEffects:0})
   await evaluate(`document.querySelector('[data-draft-support] summary').click()`)
   await wait(`document.querySelector('[data-draft-support]').open`)
   assert.ok((await evaluate(`document.querySelector('[data-draft-support]').innerText`)).includes('Java'))
   await evaluate(`(()=>{const input=document.querySelector('[data-draft-job]');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(input,'Unsaved browser wording');input.dispatchEvent(new Event('input',{bubbles:true}))})()`)
   await control('expire')
   await evaluate(`document.querySelector('[data-draft-recover]').click()`)
   await wait(`document.querySelector('[role="alert"]')?.textContent.includes(${JSON.stringify(locale==='en'?'Sign in again':'Entre novamente')})`)
   assert.equal(await evaluate(`document.querySelector('[data-draft-job]').value`),'Unsaved browser wording')
   await control('reauth')
   await evaluate(`document.querySelector('[data-draft-recover]').click()`)
   await wait(`!document.querySelector('[role="alert"]')&&!document.querySelector('[data-draft-recover]').disabled`)
   assert.equal(await evaluate(`document.querySelector('[data-draft-job]').value`),'Unsaved browser wording')
   assert.deepEqual(await control('stats','GET'),{generation:1,completion:1,allowanceEffects:0})
   await control('profile-change')
   await evaluate(`document.querySelector('[data-draft-recover]').click()`)
   await wait(`draftWire.responses.some(r=>r.body.historical===true)`)
   const historical=await evaluate(`draftWire.responses.findLast(r=>r.body.historical===true).body`)
   assert.deepEqual(historical.bundle,initial.bundle)
   assert.ok((await evaluate(`document.querySelector('[data-draft-result]').innerText`)).includes(locale==='en'?'Profile has changed':'Perfil mudou'))
   assert.equal(await evaluate(`draftWire.requests.filter(r=>r.path==='/api/profile/current'&&r.method==='PUT').length`),0)
   assert.ok(!await evaluate(`JSON.stringify(Object.entries(sessionStorage).filter(([k])=>k.startsWith('careeros_draft_recovery:'))).includes('Java required.')`))
   assert.ok(!await evaluate(`JSON.stringify(Object.entries(sessionStorage).filter(([k])=>k.startsWith('careeros_draft_recovery:'))).includes('Original Person')`))
   const switched=await control('switch');assert.notEqual(switched.account,initial.action.owner)
   const beforeSwitchReload=await evaluate(`draftWire.documentID`)
   await call('Page.reload',{ignoreCache:true})
   await wait(`window.draftWire?.documentID && draftWire.documentID!==${JSON.stringify(beforeSwitchReload)} && document.querySelector('header nav')&&!document.querySelector('[data-draft-result]')`)
   assert.equal(await evaluate(`draftWire.requests.filter(r=>r.path.startsWith('/api/drafts/')&&r.path.includes('/versions/')).length`),0)
   assert.deepEqual(await control('stats','GET'),{generation:1,completion:1,allowanceEffects:0})
   console.log(JSON.stringify({status:'passed',locale,evidence,checks:['actual React→Vite proxy→Go authenticated controlled router→PostgreSQL','lost creation reply after real publication','byte-value exact full DTO reload','generation1/check1 remain unchanged','expiry/reauth preserves unsaved text','historical original signature/source','foreign account isolation','zero Profile/Accept/allowance writes','opaque-key-only browser storage']}))
  },{path:'/tests/draft-recovery-harness.html',env:{VITE_CAREEROS_DRAFT_RECOVERY_MODE:'1',CAREEROS_API_TARGET:api},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(locale)});window.draftControlToken=${JSON.stringify(token)};window.dropDraftResponse=true`,beforeNavigate:async({port})=>{
   const file=join(evidence,'config.json');writeFileSync(file,JSON.stringify({Origin:`http://127.0.0.1:${port}`,Address:`127.0.0.1:${apiPort}`,Token:token}),{mode:0o600})
   const log=createWriteStream(join(evidence,'server.jsonl'),{flags:'wx'})
   server=spawn('rtk',['proxy','go','test','-json','-vet=off','-count=1','-run','^TestControlledDraftBrowserServer$','-timeout=180s','./internal/accountapi'],{cwd:root,env:{...process.env,CAREEROS_DRAFT_BROWSER_CONFIG:file,GOCACHE:'/tmp/careeros-go-cache'},stdio:['ignore','pipe','pipe']});server.stdout.pipe(log);server.stderr.pipe(log);serverExit=new Promise(resolve=>server.on('exit',code=>{log.end();resolve(code)}))
   for(let i=0;i<150;i++){try{if((await fetch(`${api}/api/drafts/test/health?token=${token}`)).ok)return}catch{};if(server.exitCode!==null)throw new Error('Private Go browser server exited; see '+evidence);await pause(100)}
   throw new Error('Private Go browser server timed out; see '+evidence)
  }})
 }finally{
  if(server){try{await fetch(`${api}/api/drafts/test/stop?token=${token}`)}catch{};const code=await Promise.race([serverExit,pause(10000).then(()=>{server.kill('SIGTERM');return 'terminated'})]);if(code!==0)console.error(JSON.stringify({serverExit:code,evidence}))}
 }
}
