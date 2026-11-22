import assert from "node:assert/strict"
import {spawn} from "node:child_process"
import {mkdtempSync,writeFileSync,createWriteStream} from "node:fs"
import {tmpdir} from "node:os"
import {join} from "node:path"
import {createServer} from "node:net"
import {randomUUID} from "node:crypto"
import {withAccountBrowser,pause} from "./account-browser.fixture.mjs"
const root=new URL("..",import.meta.url).pathname
const port=()=>new Promise(resolve=>{const s=createServer();s.listen(0,"127.0.0.1",()=>{const n=s.address().port;s.close(()=>resolve(n))})})
const selected=process.env.CAREEROS_LIFECYCLE_BROWSER_CELL
const cells=selected?(selected==="career-denied"?[["en",selected],["pt-BR",selected]]:[["en",selected]]):[["en","notice"],["en","cleanup"],["pt-BR","notice"],["pt-BR","cleanup"]]
for(const [locale,kind] of cells){
 const apiPort=await port(),api=`http://127.0.0.1:${apiPort}`,token=randomUUID(),evidence=mkdtempSync(join(tmpdir(),"careeros-lifecycle-browser-"));let server,exit
 try{
  await withAccountBrowser(async({call,evaluate})=>{
   const wait=async(expression)=>{for(let i=0;i<400;i++){try{if(await evaluate(`Boolean(${expression})`))return}catch(e){if(!/(Execution context|Inspected target|Cannot find context)/.test(String(e)))throw e}await pause(25)}throw Error("Timed out: "+expression)}
   const control=name=>evaluate(`fetch('/api/pilot/deletion-test/'+${JSON.stringify(name)}+'?token='+encodeURIComponent(deletionToken),{method:'POST'}).then(r=>{if(!r.ok)throw Error('private control failed');return ${JSON.stringify(name)}==='snapshot'?r.json():null})`)
   const press=async(selector="[data-lifecycle-refresh]")=>{await call("Page.bringToFront");await pause(150);await wait(`document.querySelector(${JSON.stringify(selector)})`);await evaluate(`document.querySelector(${JSON.stringify(selector)}).focus()`);assert.equal(await evaluate(`document.activeElement===document.querySelector(${JSON.stringify(selector)})`),true);await call("Input.dispatchKeyEvent",{type:"keyDown",key:"Enter",code:"Enter",windowsVirtualKeyCode:13,text:"\r"});await call("Input.dispatchKeyEvent",{type:"keyUp",key:"Enter",code:"Enter",windowsVirtualKeyCode:13})}
   await wait(`document.querySelectorAll('[data-lifecycle-notice]').length===${["late-export","career-denied"].includes(kind)?1:2}`)
   if(kind==="career-denied"){
    await evaluate(`(async()=>{const owner=deletionWire.session.account.id;const doc=(await fetch('/api/profile/current',{headers:{'X-CareerOS-Account':owner}}).then(r=>r.json())).document;const expected=doc.revision;doc.revision++;for(const f of doc.facts)if(f.owner.id===doc.id)f.owner.revision=doc.revision;const f=doc.facts[0];f.value='UNSAVED OWNER WORDING';f.revision++;f.origin={kind:'manual_edit',original:'user'};f.approval='approved';f.support='invalidated';f.normalization={observed:f.value,canonical:null,policy:'exact-alias-v1'};window.pendingOwner=owner;sessionStorage.setItem('careeros_pending_profile_v1:'+owner,JSON.stringify({version:1,accountId:owner,pendingSave:{operationId:'browser-pending',expectedRevision:expected,document:doc}}));location.reload()})()`)
    await wait(`document.querySelector('[data-sign-out]')&&document.querySelector('[data-lifecycle-notice]')`)
    await wait(`document.querySelector('header nav')`)
    await evaluate(`[...document.querySelectorAll('header nav button')].find(b=>['PROFILE','PERFIL'].includes(b.textContent.trim().toUpperCase())).click()`)
    await wait(`document.querySelector('[data-profile-export-download]')`)
    await evaluate(`window.holdExport=true;window.reviewDownloads=0;const create=URL.createObjectURL.bind(URL);URL.createObjectURL=(...args)=>{window.reviewDownloads++;return create(...args)};document.querySelector('[data-profile-export-download]').click()`)
    await wait(`typeof window.releaseExport==='function'`)
    await evaluate(`document.querySelector('[data-sign-out]').click()`)
    await wait(`document.querySelector('[data-pending-logout]')&&document.querySelector('[data-logout-download]')`)
    await evaluate(`window.reviewFocusCount=0;window.addEventListener('focus',()=>window.reviewFocusCount++);const prior=window.fetch;window.fetch=async(...args)=>{const r=await prior(...args);if(String(args[0])==='/api/lifecycle/status')window.reviewLifecycle=await r.clone().json();return r}`)
    await control("pruning-read-deny");await evaluate(`document.querySelector('[data-lifecycle-refresh]').click()`)
    await wait(`window.reviewLifecycle?.careerReadAllowed===false`);await pause(100)
    await evaluate(`document.querySelector('[data-logout-download]')?.click()`)
    const observed=await evaluate(`({focusEvents:reviewFocusCount,careerReadAllowed:reviewLifecycle.careerReadAllowed,contentExpired:reviewLifecycle.contentExpired,metadataVisible:Boolean(document.querySelector('[data-account-lifecycle]')),downloadOffered:Boolean(document.querySelector('[data-logout-download]')),downloads:reviewDownloads})`)
    console.log(JSON.stringify({review:"direct metadata refresh without focus",observed,evidence}))
    assert.equal(observed.focusEvents,0,"focus/probe contaminated direct metadata refresh")
    assert.equal(observed.contentExpired,false,"ordinary read fence, not whole-window expiry")
    assert.equal(observed.metadataVisible,true)
    assert.equal(observed.downloadOffered,false,"known career-read denial from lifecycle refresh leaves protected merged recovery download")
    assert.equal(observed.downloads,0,"protected pending export performed after known read denial")
    assert.equal(await evaluate(`[...document.querySelectorAll('button')].some(b=>!b.closest('[hidden]')&&/download|baixar/i.test(b.textContent))`),false,"another protected recovery download remains visible")
    await evaluate(`window.releaseExport()`);await pause(150)
    assert.equal(await evaluate(`window.reviewDownloads`),0,"held native export created a download after direct metadata denial")
    assert.equal(await evaluate(`Boolean(sessionStorage.getItem('careeros_pending_profile_v1:'+deletionWire.session.account.id))`),true,"independent pending input/key was erased")
    await control("pruning-read-clear");await evaluate(`window.reviewLifecycle=null;document.querySelector('[data-lifecycle-refresh]').click()`)
    await wait(`window.reviewLifecycle?.careerReadAllowed===true`);await pause(100)
    assert.equal(await evaluate(`Boolean(document.querySelector('[data-logout-download]'))`),false,"metadata alone restored protected read access")
    await evaluate(`window.dispatchEvent(new Event('focus'))`)
    await wait(`document.querySelector('[data-logout-download]')`)
    assert.equal(await evaluate(`deletionWire.calls.some(x=>x.method==='PUT')`),false,"recovery auto-saved")
    const oldOwner=await evaluate(`deletionWire.session.account.id`)
    await control("switch");await evaluate(`window.dispatchEvent(new Event('focus'))`)
    await wait(`deletionWire.session.account.id!==${JSON.stringify(oldOwner)}&&document.querySelector('[data-sign-out]')&&!document.querySelector('[data-sign-out]').closest('[hidden]')`)
    assert.equal(await evaluate(`Boolean(document.querySelector('[data-pending-logout]'))||document.body.textContent.includes('UNSAVED OWNER WORDING')`),false,"foreign scope inherited protected pending work")
    console.log(JSON.stringify({status:"passed",locale,kind,evidence,checks:["direct lifecycle GET refresh denies career reads with zero focus events","metadata and independent pending input remain accessible","merged pending download and held native export canceled immediately","positive metadata alone cannot restore protected access","fresh same-owner authority permits deliberate recovery/zero automatic Save or inference","foreign scope receives no prior pending result or wording"],limits:["private synthetic pruning fence/reconciliation","test-injected account-scoped pending checkpoint"]}));return
   }
   if(kind==="late-export"){
    await wait(`document.querySelector('header nav')`)
    await evaluate(`[...document.querySelectorAll('header nav button')].find(b=>b.textContent.trim().toUpperCase()==='PROFILE').click()`)
    await wait(`document.querySelector('[data-profile-export-download]')`)
    await evaluate(`window.holdExport=true;window.createdDownloads=0;const create=URL.createObjectURL.bind(URL);URL.createObjectURL=(...args)=>{window.createdDownloads++;return create(...args)};document.querySelector('[data-profile-export-download]').click()`)
    await wait(`typeof window.releaseExport==='function'`)
    await control("lifecycle-expire");await press()
    await wait(`document.querySelector('[data-lifecycle-cleanup]')?.textContent.includes('Active content removed')`)
    await evaluate(`window.releaseExport()`);await pause(300)
    assert.equal(await evaluate(`window.createdDownloads`),0,"known same-owner removal allowed a late protected download")
    console.log(JSON.stringify({status:"passed",locale,kind,evidence,checks:["actual previously authorized native export response held","known same-owner window expiry/primary cleanup metadata","late response invalidated before a new download","zero inference/unchanged already-held downloads"],limits:["test-only clock transition","no real recipient operations"]}));return
   }
   const before=await control("snapshot");assert.equal(before.primary,1);assert.equal(before.posts,0);assert.equal(before.lifecycleEmails,0)
   const original=await evaluate(`Array.from(document.querySelectorAll('[data-lifecycle-notice] time'),e=>e.dateTime)`)
   const text=await evaluate(`document.querySelector('[data-account-lifecycle]').textContent`)
   assert.ok(text.includes(locale==="en"?"Pilot access ended":"O acesso ao piloto terminou"))
   await control("lifecycle-run");await press()
   await wait(`document.querySelector('[data-lifecycle-email]')?.textContent.includes(${JSON.stringify(locale==="en"?"deadline is unchanged":"prazo não muda")})`)
   if(kind==="cleanup")await wait(`document.querySelector('[data-lifecycle-cleanup]')?.textContent.includes(${JSON.stringify(locale==="en"?"Active content removed":"Conteúdo ativo removido")})`)
   assert.deepEqual(await evaluate(`Array.from(document.querySelectorAll('[data-lifecycle-notice] time'),e=>e.dateTime)`),original)
   const after=await control("snapshot");assert.equal(after.posts,0);assert.equal(after.primary,kind==="cleanup"?0:1);assert.ok(after.lifecycleEmails>0);assert.equal(after.accountState,"active")
   assert.equal(await evaluate(`deletionWire.calls.some(x=>x.method==='POST'&&!x.path.includes('/deletion-test/'))`),false,"status/notice UI caused a mutation")
   await evaluate(`location.reload()`);await wait(`document.querySelectorAll('[data-lifecycle-notice]').length===2`)
   if(kind==="cleanup"){await wait(`document.querySelector('[data-lifecycle-residual]')`);assert.ok((await evaluate(`document.querySelector('[data-lifecycle-residual]').textContent`)).includes(locale==="en"?"not verified":"não foi verificada"))}
   console.log(JSON.stringify({status:"passed",locale,kind,evidence,checks:["actual React App/Vite/authenticated Go/private PostgreSQL","metadata GET presents original expiry/removal notices before worker","fake email failure remains visible and records operator signal","fixed original deadline/no extension","explicit trusted-keyboard status refresh/GET only","reload preserves metadata and no AI, deletion POST or allowance effect",kind==="cleanup"?"bounded worker removed primary content; residual not verified":"retained Profile survives before window ends"],limits:["test-only SQL anchor schedule","code-injected fake email; no real delivery/scheduler","local fixture, not hosted disposal"]}))
  },{path:"/tests/account-deletion-harness.html",env:{VITE_CAREEROS_ACCOUNT_MODE:"1",VITE_CAREEROS_PILOT_MODE:"1",CAREEROS_API_TARGET:api},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(locale)});window.deletionToken=${JSON.stringify(token)}`,beforeNavigate:async({port})=>{
   const config=join(evidence,"config.json");writeFileSync(config,JSON.stringify({Origin:`http://127.0.0.1:${port}`,Address:`127.0.0.1:${apiPort}`,Token:token}),{mode:0o600});const log=createWriteStream(join(evidence,"server.jsonl"),{flags:"wx"})
   server=spawn("rtk",["proxy","go","test","-json","-vet=off","-count=1","-run","^TestDeletionBrowserServer$","-timeout=180s","./internal/accountapi"],{cwd:root,env:{...process.env,CAREEROS_DELETION_BROWSER_CONFIG:config,CAREEROS_LIFECYCLE_BROWSER_KIND:kind,GOCACHE:"/tmp/careeros-go-cache"},stdio:["ignore","pipe","pipe"]});server.stdout.pipe(log);server.stderr.pipe(log);exit=new Promise(resolve=>server.on("exit",code=>{log.end();resolve(code)}))
   for(let i=0;i<200;i++){try{if((await fetch(`${api}/api/pilot/deletion-test/health?token=${token}`)).ok)return}catch{};if(server.exitCode!==null)throw Error("private server exited: "+evidence);await pause(100)}throw Error("private server timeout: "+evidence)
  }})
 }finally{if(server){try{await fetch(`${api}/api/pilot/deletion-test/stop?token=${token}`,{method:"POST"})}catch{};const code=await Promise.race([exit,pause(10000).then(()=>{server.kill("SIGTERM");return"terminated"})]);assert.equal(code,0,"private server terminal status")}}
}
