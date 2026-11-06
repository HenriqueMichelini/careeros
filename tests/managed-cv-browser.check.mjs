import assert from 'node:assert/strict'
import {withAccountBrowser} from './account-browser.fixture.mjs'
for(const pt of [false,true]){
 const label={cv:pt?'Currículo':'CV',generate:pt?'Gerar CV':'Generate CV',accept:pt?'Aceitar e usar este CV':'Accept and use this CV'}
 const by=t=>`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(t)})`
 const open=async({evaluate,wait})=>{await wait(`document.querySelector('header nav')`);await evaluate(`${by(label.cv)}.click()`);await wait(`document.querySelector('[data-cv-managed-status]')`)}
 await withAccountBrowser(async({evaluate,wait,evidence})=>{
  await open({evaluate,wait})
  const generate=`[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(pt?'Gerar':'Generate')})&&!b.disabled)`
  await evaluate(`${generate}.click()`);await wait(`document.querySelector('[data-cv-review]')`)
  let posts=await evaluate(`managedCVScenario.posts`);assert.equal(posts.length,1);assert.equal(posts[0].headers['x-openai-api-key'],undefined);assert.ok(posts[0].headers['x-careeros-action-key']);assert.equal(posts[0].body.statementSupport,true)
  assert.equal(await evaluate(`managedCVScenario.profileWrites`),0)
  const accept=`[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(pt?'Aceitar':'Accept')})&&b.closest('[data-cv-review]')===null)`
  await wait(`document.querySelector('[data-cv-review-blocked]')`)
  const acceptButtons=await evaluate(`[...document.querySelectorAll('button')].filter(b=>b.textContent.includes(${JSON.stringify(pt?'Aceitar':'Accept')})).map(b=>({text:b.textContent,disabled:b.disabled}))`);assert.ok(acceptButtons.some(b=>b.disabled))
  await evaluate(`document.querySelector('[data-cv-review-edit-toggle]').click()`)
  await wait(`document.querySelector('[data-cv-review-edit-input]')`)
  await evaluate(`(()=>{const i=document.querySelector('[data-cv-review-edit-input]');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(i,'User-authored Java summary');i.dispatchEvent(new Event('input',{bubbles:true}))})()`)
  await wait(`![...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(pt?'Aceitar':'Accept')}))?.disabled`)
  await evaluate(`[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(pt?'Aceitar':'Accept')})).click()`)
  await wait(`localStorage.getItem('careeros_account:synthetic-a:careeros_curated_cv_v1')`)
  const saved=await evaluate(`JSON.parse(localStorage.getItem('careeros_account:synthetic-a:careeros_curated_cv_v1'))`);assert.equal(saved.version,3);assert.ok(JSON.stringify(saved).includes('User-authored Java summary'));assert.equal(await evaluate(`managedCVScenario.posts.length`),1)
  assert.equal(await evaluate(`managedCVScenario.profileWrites`),0)
  await evaluate(`document.querySelector('[data-cv-managed-status]').click()`);await wait(`managedCVScenario.reads===1`);assert.equal(await evaluate(`managedCVScenario.posts.length`),1)
  assert.ok(!await evaluate(`JSON.stringify(Object.entries(sessionStorage)).includes('User-authored Java summary')`))
  console.log(JSON.stringify({status:'passed',locale:pt?'pt-BR':'en',evidence,checks:['actual App pilot key-free CV','unavailable candidate retained','whole Accept blocked','per-statement user edit','local atomic Accept','zero Draft/Profile writes','GET0sends']}))
 },{path:'/tests/managed-cv-harness.html',env:{VITE_CAREEROS_PILOT_MODE:'1'},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(pt?'pt-BR':'en')});window.cvMode='uncertain'`})
 await withAccountBrowser(async({evaluate,wait,evidence})=>{
  await open({evaluate,wait});const generate=`[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(pt?'Gerar':'Generate')})&&!b.disabled)`
  await evaluate(`${generate}.click()`);await wait(`managedCVScenario.posts.length===1&&!${generate}?.disabled`)
  const first=(await evaluate(`managedCVScenario.posts`))[0]
  await evaluate(`document.querySelector('header nav button').click()`);await open({evaluate,wait})
  await evaluate(`document.querySelector('[data-cv-managed-status]').click()`);await wait(`managedCVScenario.reads===1`);assert.equal(await evaluate(`managedCVScenario.posts.length`),1)
  await evaluate(`managedCVScenario.mode='known_failure';document.querySelector('[data-cv-managed-status]').click()`);await wait(`managedCVScenario.reads===2`)
  await evaluate(`[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(pt?'Tentar novamente':'Retry failed')})).click()`);await wait(`document.querySelector('[data-cv-review]')`)
  const posts=await evaluate(`managedCVScenario.posts`);assert.equal(posts.length,2);assert.equal(posts[1].headers['x-careeros-action-key'],first.headers['x-careeros-action-key']);assert.ok(posts[1].headers['x-careeros-retry-key']);assert.deepEqual(posts[1].body,first.body)
  console.log(JSON.stringify({status:'passed',locale:pt?'pt-BR':'en',evidence,checks:['actual sparse education CV','unknown navigation GET0sends','deliberate known generation Retry unchangedroot','no candidate recovery promise']}))
 },{path:'/tests/managed-cv-harness.html',env:{VITE_CAREEROS_PILOT_MODE:'1'},initialize:`localStorage.setItem('careeros_locale',${JSON.stringify(pt?'pt-BR':'en')});window.cvMode='unknown';window.cvSparse=true`})
}
